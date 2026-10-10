#!/usr/bin/env bash
# A local, production-like servly demo, set up the way an operator would
# (docs/docs/setup.md): fresh data directory, restaurant and owner created
# with `servly tenant create`, then menu, tables and staff through the real
# API, one sample guest order, and the server left running.
#
# usage: make demo            (builds first)
#        scripts/demo.sh      (uses dist/servly as built)
#
# Environment:
#   DEMO_DIR    data directory, wiped on every run   (default: ./.demo)
#   DEMO_PORT   first port to try; the next free one is used (default: 8080)
#   DEMO_HOST   address guests' phones use for QR codes (default: this host's LAN IP)

set -euo pipefail

BIN="${BIN:-dist/servly}"
DIR="${DEMO_DIR:-./.demo}"
PORT="${DEMO_PORT:-8080}"
HOST="${DEMO_HOST:-}"

die() { echo "demo: $*" >&2; exit 1; }
command -v curl >/dev/null || die "curl is required"
[[ -x "$BIN" ]] || die "$BIN not found; run 'make build' (or 'make demo')"

# --- pick a free port -------------------------------------------------------
port_busy() { (echo >"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
for _ in $(seq 1 20); do
  port_busy "$PORT" || break
  PORT=$((PORT + 1))
done
port_busy "$PORT" && die "no free port near ${DEMO_PORT:-8080}; set DEMO_PORT"

# --- where guests reach the server -----------------------------------------
if [[ -z "$HOST" ]]; then
  HOST="$(hostname -I 2>/dev/null | awk '{print $1}')" || true
  [[ -n "$HOST" ]] || HOST="$(ipconfig getifaddr en0 2>/dev/null || true)"
  [[ -n "$HOST" ]] || HOST=localhost
fi
LISTEN="0.0.0.0:$PORT"
[[ "$HOST" == localhost || "$HOST" == 127.* ]] && LISTEN="127.0.0.1:$PORT"
BASE="http://$HOST:$PORT"
LOCAL="http://127.0.0.1:$PORT"

# Every reader below consumes its whole input: under pipefail, a reader that
# stops early (head) would kill the script with SIGPIPE.
password() { head -c 48 /dev/urandom | base64 | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-16; }
OWNER_EMAIL=owner@example.com   OWNER_PW="$(password)"
MANAGER_EMAIL=manager@example.com MANAGER_PW="$(password)"
STAFF_EMAIL=staff@example.com   STAFF_PW="$(password)"

export SERVLY_DATA_DIR="$DIR" SERVLY_LISTEN_ADDR="$LISTEN" SERVLY_HTTP_PUBLIC_BASE_URL="$BASE" \
  SERVLY_LOG_FORMAT=text SERVLY_LOG_LEVEL=warn

# --- 1. fresh data, restaurant and owner (setup guide step 3) ---------------
echo "==> fresh data directory $DIR"
rm -rf "$DIR"
mkdir -p "$DIR"
echo "==> creating the restaurant and its owner"
echo "$OWNER_PW" | "$BIN" tenant create --name "Trattoria Demo" --currency EUR --timezone Europe/Berlin \
  --owner-email "$OWNER_EMAIL" --owner-name "Olivia Owner" --password-stdin >/dev/null

# --- 2. start the server (step 2) -------------------------------------------
echo "==> starting servly on $LISTEN"
"$BIN" serve >"$DIR/servly.log" 2>&1 &
PID=$!
# Stop the server however the script ends: error, Ctrl-C or kill.
trap 'kill "$PID" 2>/dev/null; wait "$PID" 2>/dev/null' EXIT
trap 'exit 130' INT TERM
for _ in $(seq 1 50); do
  curl -fs "$LOCAL/readyz" >/dev/null 2>&1 && break
  kill -0 "$PID" 2>/dev/null || { cat "$DIR/servly.log" >&2; die "server did not start"; }
  sleep 0.2
done
curl -fs "$LOCAL/readyz" >/dev/null || die "server not ready; see $DIR/servly.log"

# --- 3. sign in as the owner and build the restaurant (steps 4-8) -----------
JAR="$DIR/cookies"
api() { # api METHOD PATH [JSON] -> response body; fails on HTTP errors
  local out code
  out="$(curl -s -w '\n%{http_code}' -b "$JAR" -c "$JAR" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRF:-}" -X "$1" "$LOCAL$2" ${3:+--data "$3"})"
  code="${out##*$'\n'}"
  out="${out%$'\n'*}"
  [[ "$code" == 2* ]] || die "$1 $2 -> $code $out"
  printf '%s' "$out"
}
field() { grep -o "\"$1\":\"[^\"]*\"" | sed -n 1p | cut -d'"' -f4; } # first "field":"value"

echo "==> signing in as the owner"
CSRF=""
CSRF="$(api POST /api/auth/login "{\"email\":\"$OWNER_EMAIL\",\"password\":\"$OWNER_PW\"}" | field csrf_token)"

echo "==> menu"
category() { api POST /api/admin/menu/categories "{\"name\":\"$1\"}" | field id; }
item() { api POST /api/admin/menu/items "$1" | field id; }
size='{"name":"Size","min_select":1,"max_select":1,"options":[{"name":"Normal","price_delta_minor":0},{"name":"Large","price_delta_minor":300}]}'
extras='{"name":"Extras","min_select":0,"max_select":2,"options":[{"name":"Basil","price_delta_minor":50},{"name":"Burrata","price_delta_minor":250},{"name":"Chili oil","price_delta_minor":0}]}'
PIZZA="$(category Pizza)"
PASTA="$(category Pasta)"
DRINKS="$(category Drinks)"
item "{\"category_id\":\"$PIZZA\",\"name\":\"Margherita\",\"description\":\"Tomato, mozzarella, basil\",\"price_minor\":950,\"option_groups\":[$size,$extras]}" >/dev/null
item "{\"category_id\":\"$PIZZA\",\"name\":\"Diavola\",\"description\":\"Spicy salami, chili\",\"price_minor\":1150,\"option_groups\":[$size,$extras]}" >/dev/null
item "{\"category_id\":\"$PASTA\",\"name\":\"Carbonara\",\"description\":\"Guanciale, egg, pecorino\",\"price_minor\":1250}" >/dev/null
item "{\"category_id\":\"$PASTA\",\"name\":\"Arrabbiata\",\"description\":\"Tomato, garlic, chili\",\"price_minor\":1050}" >/dev/null
COLA="$(item "{\"category_id\":\"$DRINKS\",\"name\":\"Cola\",\"price_minor\":300}")"
item "{\"category_id\":\"$DRINKS\",\"name\":\"Sparkling water\",\"price_minor\":250}" >/dev/null
item "{\"category_id\":\"$DRINKS\",\"name\":\"Lemonade\",\"description\":\"Homemade\",\"price_minor\":350,\"is_available\":false}" >/dev/null

echo "==> tables"
TABLE_URLS=()
for name in "Table 1" "Table 2" "Table 3" "Table 4"; do
  TABLE_URLS+=("$name|$(api POST /api/admin/dining-locations "{\"name\":\"$name\",\"kind\":\"table\"}" | field menu_url)")
done
PICKUP="$(api POST /api/admin/dining-locations '{"name":"Pickup counter","kind":"pickup"}')"
TABLE_URLS+=("Pickup counter|$(printf '%s' "$PICKUP" | field menu_url)")
TABLE1_TOKEN="$(printf '%s' "${TABLE_URLS[0]#*|}" | sed 's#.*/m/##')"

echo "==> staff"
api POST /api/admin/staff "{\"email\":\"$MANAGER_EMAIL\",\"display_name\":\"Marco Manager\",\"role\":\"manager\",\"password\":\"$MANAGER_PW\"}" >/dev/null
api POST /api/admin/staff "{\"email\":\"$STAFF_EMAIL\",\"display_name\":\"Sam Staff\",\"role\":\"staff\",\"password\":\"$STAFF_PW\"}" >/dev/null
api POST /api/auth/logout >/dev/null

# --- 4. a guest orders, as a phone would (step 9) ---------------------------
echo "==> a guest at Table 1 places an order"
ORDER_BODY="{\"dining_location_token\":\"$TABLE1_TOKEN\",\"notes\":\"demo order\",\"items\":[{\"menu_item_id\":\"$COLA\",\"quantity\":2,\"option_ids\":[],\"notes\":\"no ice\"}]}"
CSRF=""
ORDER="$(curl -fsS -H 'Content-Type: application/json' -H "Idempotency-Key: demo-$(password)" \
  --data "$ORDER_BODY" "$LOCAL/api/public/orders")" || die "sample order failed"
ORDER_NO="$(printf '%s' "$ORDER" | grep -o '"number":[0-9]*' | cut -d: -f2)"
ORDER_TOKEN="$(printf '%s' "$ORDER" | field order_token)"

# --- 5. what to open --------------------------------------------------------
cat <<EOF

────────────────────────────────────────────────────────────────────────
 servly demo is running: $BASE   (data: $DIR, log: $DIR/servly.log)
────────────────────────────────────────────────────────────────────────

 Staff sign-in   $BASE/login
   owner     $OWNER_EMAIL     $OWNER_PW
   manager   $MANAGER_EMAIL   $MANAGER_PW
   staff     $STAFF_EMAIL     $STAFF_PW

 Order board     $BASE/staff        (order #$ORDER_NO from Table 1 is waiting)
 Administration  $BASE/admin        (QR sheet: $BASE/admin/tables/print)

 Guest menus (what the QR codes open):
EOF
for t in "${TABLE_URLS[@]}"; do printf '   %-15s %s\n' "${t%%|*}" "${t#*|}"; done
cat <<EOF

 The sample guest order:  $BASE/o/$ORDER_TOKEN

 Order as a guest from the command line (what the cart page sends):
   curl -s $BASE/api/public/orders \\
     -H 'Content-Type: application/json' -H "Idempotency-Key: try-\$RANDOM\$RANDOM" \\
     --data '$ORDER_BODY'

EOF
if [[ "$LISTEN" == 0.0.0.0:* ]]; then
  echo " Phones on the same network can scan the QR codes (the server listens on all interfaces)."
  echo " If they cannot reach $HOST (WSL, VPN, firewall), set DEMO_HOST to an address they can."
fi
echo " Ctrl-C stops the demo. Run 'make demo' again for a fresh one."
echo
wait "$PID"
