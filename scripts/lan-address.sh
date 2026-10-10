#!/usr/bin/env bash
# Print the address other devices on the local network use to reach PORT on
# this machine, and make the port reachable first where that needs a step.
#
# Linux and macOS: the machine's LAN address; nothing to do.
# WSL 2 in mirrored networking mode: the Windows LAN address; nothing to do.
# WSL 2 in NAT mode (the default): Linux ports are reachable from Windows only.
#   The script forwards PORT on Windows to this WSL instance (netsh portproxy)
#   and allows it in the Windows firewall. That takes one administrator prompt
#   on Windows, and only when the forward is missing or points at an old WSL
#   address (it changes when WSL restarts).
#
# The firewall rule covers Private and Domain networks only. A network that
# Windows marks as Public stays closed unless LAN_ALLOW_PUBLIC=1 is set.
#
# usage: scripts/lan-address.sh PORT            prints the address on stdout
#        scripts/lan-address.sh --remove PORT   removes the forward and the rule
#
# Notes go to stderr. The script never fails the caller: without a usable LAN
# address it prints this machine's best guess, or localhost.

set -uo pipefail

remove=0
if [[ "${1:-}" == --remove ]]; then
  remove=1
  shift
fi
PORT="${1:?usage: lan-address.sh [--remove] PORT}"
RULE="servly $PORT"

note() { echo "lan: $*" >&2; }
linux_ip() { hostname -I 2>/dev/null | awk '{print $1}'; }

if ! grep -qi microsoft /proc/version 2>/dev/null || ! command -v powershell.exe >/dev/null; then
  ip="$(linux_ip)"
  [[ -n "$ip" ]] || ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
  echo "${ip:-localhost}"
  exit 0
fi

ps() { powershell.exe -NoProfile -NonInteractive -Command "$1" 2>/dev/null | tr -d '\r'; }

# Run a PowerShell script as administrator: Windows shows one UAC prompt.
ps_admin() {
  local enc
  enc="$(printf '%s' "$1" | iconv -t UTF-16LE | base64 -w0)"
  powershell.exe -NoProfile -Command \
    "Start-Process powershell -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-EncodedCommand','$enc'" >/dev/null 2>&1
}

if ((remove)); then
  ps_admin "netsh interface portproxy delete v4tov4 listenport=$PORT listenaddress=0.0.0.0; Remove-NetFirewallRule -DisplayName '$RULE' -ErrorAction SilentlyContinue" \
    && note "removed the Windows forward and firewall rule for port $PORT" \
    || note "not removed (administrator prompt declined?)"
  exit 0
fi

win_ip="$(ps '(Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq "Up" } | Select-Object -First 1).IPv4Address.IPAddress' | head -n1)"
wsl_ip="$(linux_ip)"
if [[ -z "$win_ip" ]]; then
  note "no Windows LAN address found; other devices cannot reach this WSL instance"
  echo "${wsl_ip:-localhost}"
  exit 0
fi

mode="$(wslinfo --networking-mode 2>/dev/null || echo nat)"
if [[ "$mode" != nat ]]; then
  # Mirrored: WSL shares the Windows addresses; ports bound on 0.0.0.0 are already on the LAN.
  echo "$win_ip"
  exit 0
fi

profiles="Private,Domain"
[[ "${LAN_ALLOW_PUBLIC:-0}" == 1 ]] && profiles="Private,Domain,Public"

current="$(ps "netsh interface portproxy show v4tov4" | awk -v p="$PORT" -v ip="$wsl_ip" '$1=="0.0.0.0" && $2==p && $3==ip && $4==p {print "ok"}')"
# The rule's profiles read back as e.g. "Domain, Private"; what matters is whether Public is in.
rule="$(ps "(Get-NetFirewallRule -DisplayName '$RULE' -ErrorAction SilentlyContinue).Profile")"
rule_ok=0
if [[ -n "$rule" ]]; then
  [[ "$rule" == *Public* || "$rule" == Any ]] && has_public=1 || has_public=0
  [[ "$has_public" == "${LAN_ALLOW_PUBLIC:-0}" ]] && rule_ok=1
fi
if [[ "$current" == ok && "$rule_ok" == 1 ]]; then
  note "port $PORT is forwarded from Windows ($win_ip) to WSL ($wsl_ip)"
else
  note "forwarding port $PORT from Windows ($win_ip) to WSL ($wsl_ip): confirm the administrator prompt on Windows"
  if ps_admin "netsh interface portproxy delete v4tov4 listenport=$PORT listenaddress=0.0.0.0 | Out-Null
netsh interface portproxy add v4tov4 listenport=$PORT listenaddress=0.0.0.0 connectport=$PORT connectaddress=$wsl_ip
Remove-NetFirewallRule -DisplayName '$RULE' -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName '$RULE' -Direction Inbound -Protocol TCP -LocalPort $PORT -Action Allow -Profile $profiles | Out-Null"; then
    note "done; undo with: make lan-close PORT=$PORT"
  else
    note "not forwarded (prompt declined?); only this Windows machine can open the server"
    echo "$wsl_ip"
    exit 0
  fi
fi

category="$(ps '(Get-NetConnectionProfile | Where-Object { $_.IPv4Connectivity -ne "Disconnected" } | Select-Object -First 1).NetworkCategory')"
if [[ "$category" == Public && "${LAN_ALLOW_PUBLIC:-0}" != 1 ]]; then
  note "Windows treats this network as Public, so the firewall still blocks other devices."
  note "  Trusted network (home, restaurant): Windows Settings > Network & internet > your Wi-Fi > Private network,"
  note "  or rerun with LAN_ALLOW_PUBLIC=1 to open the port on Public networks too."
fi
echo "$win_ip"
