---
id: setup
title: Setting up a restaurant
---

This guide takes one restaurant from a fresh install to its first guest order. It is written for whoever installs servly in the restaurant, whether the owner or the person who looks after their computers. Each step names the screen or command to use.

To see the result before doing it by hand, run [`make demo`](#try-it-make-demo). It performs these same steps automatically.

## 1. Install servly

Pick one way to run it, as described in [Local deployment](local-deployment.md):

* **Container:** `podman run -d --name servly --restart=always -p 8080:8080 -v servly-data:/data ghcr.io/servly-io/servly:latest`
* **Binary with systemd:** install `servly` and `deploy/systemd/servly.service`.

## 2. Make it reachable from guests' phones

Guests scan a QR code. The code holds an address their phone must be able to open.

1. Find the server's address on the restaurant network, for example `192.168.1.20`.
2. Set it as the public address, so QR codes point there:

   ```bash
   SERVLY_HTTP_PUBLIC_BASE_URL=http://192.168.1.20:8080
   ```

   Use a config file, the systemd `EnvironmentFile`, or `-e` for the container. Without this setting, QR codes use the address of the browser that printed them. That only works if the browser used the same address the guests will use.
3. Start servly and check it:

   ```bash
   curl http://192.168.1.20:8080/readyz     # {"status":"ok"}
   ```

Keep guests' Wi-Fi apart from the staff network if you can. See [Security](security.md#transport).

## 3. Create the restaurant and its owner

Run this on the server. It asks for the owner's password twice:

```bash
servly tenant create --name "Trattoria Roma" --currency EUR --timezone Europe/Berlin --owner-email anna@example.com
```

In a container:

```bash
echo "$OWNER_PASSWORD" | podman exec -i servly servly tenant create --name "Trattoria Roma" \
  --currency EUR --timezone Europe/Berlin --owner-email anna@example.com --password-stdin
```

* The **currency** applies to every price. It can still change in Settings until the first order arrives.
* The **timezone** decides when order numbers restart, at midnight local time.
* The password needs at least 10 characters.

## 4. Sign in

Open `http://<server>:8080/login` on a tablet or PC and sign in as the owner. The **Setup** page lists what is still missing.

## 5. Check the settings

**Settings** holds the name, currency and timezone, and two switches:

* **Guests can add notes to orders**, such as "no onions".
* **Guests can follow their order status** after ordering.

## 6. Build the menu

Under **Menu**:

1. **Add categories**, such as Pizza, Pasta and Drinks. Use ↑ and ↓ to set their order on the guest menu.
2. **Add items** in each category: name, description and price, entered as you would write it (`9.50`).
3. **Add option groups** where guests choose:
   * "Size": choose at least 1 and at most 1, with Normal (+0.00) and Large (+3.00).
   * "Extras": choose at least 0 and at most 3, with each extra and its surcharge.
4. **Add a photo** if you like: JPEG, PNG or WebP, up to 5 MB.
5. During service, untick **Available** for anything that sells out. Guests see it as sold out and cannot order it.

**Duplicate** saves time for similar items. **Delete** removes an item, or archives it if past orders contain it.

## 7. Add tables and print the QR codes

Under **Tables**:

1. Add one entry per table ("Table 1", "Terrace 4") and, if you sell takeaway, a **pickup point**.
2. Open **Print QR codes** and print the sheet. Each card shows the table name and its code.
3. Put each card on its table.

**New code** replaces a table's QR code, for example if a printed card was copied. The old card then stops working. Untick **Accepting orders** to pause ordering at one table.

## 8. Add your staff

Under **Staff**, add each person with an email and a starting password:

| Role | Can |
|---|---|
| staff | see orders, move them along, record payments, cancel orders not yet being prepared |
| manager | also edit the menu and tables, cancel orders being prepared, refund |
| owner | also manage staff and settings |

People who already have a servly sign-in for another restaurant are added without a password; they keep their own. If someone forgets their password, the owner sets a new one here, or the server operator runs `servly user set-password <email>`.

## 9. Test an order, then go live

1. Scan a table's QR code with a phone. The menu opens.
2. Order something. The phone shows the order number and its status.
3. On a tablet or PC, open **Orders** (`/staff`). The order appears in **New**, highlighted. Turn on **Sound** if the board should beep.
4. Move it through **Accept**, **Start preparing**, **Mark ready** and **Complete**. The guest's phone follows along.
5. Open the order, then **Record payment** (cash or card terminal).
6. Cancel or refund the test order as you would a real one.

## 10. Set up backups

Before the first busy evening, follow [Backup and restore](backup-restore.md) and run one restore test.

## Try it: `make demo`

`make demo` runs steps 2-9 on a developer machine with a fresh database, and leaves the server running until Ctrl-C:

```bash
make demo
```

In order, it:

1. builds servly (the web app and the server);
2. wipes and recreates `./.demo` (the database, uploads, `servly.log`);
3. runs `servly tenant create` for "Trattoria Demo" with a random owner password;
4. starts the server on the first free port from 8080, with this machine's network address as the public address;
5. signs in as the owner and, through the same API the admin screens use, creates a menu (pizzas with sizes and extras, pasta, drinks, one sold-out item), four tables, a pickup counter, a manager and a staff member;
6. places an order as a guest at Table 1;
7. prints the sign-ins, the board and admin addresses, every table's guest menu address, the sample order's status page, and a `curl` command that orders like the guest cart does.

| Variable | Default | |
|---|---|---|
| `DEMO_PORT` | `8080` | first port to try; the next free one is used |
| `DEMO_HOST` | this machine's LAN address | the address in QR codes; set it if phones cannot reach the default (WSL, VPN) |
| `DEMO_DIR` | `./.demo` | data directory, wiped on every run |

The demo listens on all network interfaces so phones on the same network can order. Run it only on networks you trust. Its passwords are printed on the screen.
