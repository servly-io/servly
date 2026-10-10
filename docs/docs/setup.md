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

## 3. Run the first-run setup

1. Copy the setup token from the server log. It proves that you run the server:

   ```bash
   podman logs servly 2>&1 | grep setup_token      # container
   journalctl -u servly | grep setup_token         # systemd
   ```
2. Open `http://<server>:8080` on a tablet or PC. A fresh installation opens the setup.
3. Follow the five steps:
   * **Welcome:** paste the setup token.
   * **Your account:** name, email and a password of at least 10 characters. This is the owner's sign-in.
   * **Restaurant:** name, currency and timezone. The **currency** applies to every price and can change until the first order arrives. The **timezone** decides when order numbers restart, at midnight local time.
   * **Ordering:** whether guests can add notes and follow their order status.
   * **Review:** check, then **Create restaurant**.

The setup works only once. After that it is closed, and everyone signs in at `/login`. If the server restarts before you finish, it prints a new token.

## 4. Follow the checklist

After setup, you are signed in as the owner on **Home**. Its checklist shows what is missing and which step comes next. Steps 6 to 9 below are the same steps in more detail. When everything is done, Home shows the open orders instead.

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

People who already have a servly sign-in for another restaurant are added without a password; they keep their own. If someone forgets their password, the owner sets a new one here. If the owner cannot sign in, the person who runs the server resets it ([Forgotten password](local-deployment.md#forgotten-password)).

## 9. Test an order, then go live

1. Scan a table's QR code with a phone. The menu opens.
2. Order something. The phone shows the order number and its status.
3. On a tablet or PC, open **Orders** (`/staff`). The **Kitchen** view shows the order under **New**, highlighted. Turn on the sound if the board should beep.
4. Move it through **Accept**, **Start preparing**, **Mark ready** and **Served**. The guest's phone follows along.
5. Every ticket shows whether it is paid. An order that is served but not paid moves to **To pay**, with the count shown on the button, until someone records the payment: open the order, then pick cash, card terminal or other.
6. Cancel or refund the test order as you would a real one. **History** lists completed and cancelled orders.

### Pickup orders

A pickup point works like a table, but the guest collects the order at the counter and pays there:

1. The guest scans the pickup point's QR code (on the counter, a flyer or your website) and orders. Their phone shows the order number and checks the status every few seconds.
2. Staff prepare the order as usual. When it is **Ready**, the phone buzzes and shows "Ready", and the number appears under **Ready to collect** on the pickup screen.
3. The guest shows their number at the counter. On the ticket, **Take payment & hand over** opens the order: pick how they pay, tap **Paid · hand over**, done. An order that is already paid shows **Handed over** instead.

The **pickup screen** (the screen icon on the board, `/staff/display`) is meant for a TV or tablet facing the guests. Sign it in with a staff account and open it in full screen. It shows order numbers only: **Being prepared** and **Ready to collect**.

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
3. creates "Trattoria Demo" and its owner with a random password, using `servly tenant create` as a developer shortcut for the browser setup;
4. starts the server on the first free port from 8080, with this machine's network address as the public address;
5. signs in as the owner and, through the same API the admin screens use, creates a menu (pizzas with sizes and extras, pasta, drinks, one sold-out item), four tables, a pickup counter, a manager and a staff member;
6. places an order as a guest at Table 1;
7. prints the sign-ins, the board and admin addresses, every table's guest menu address, the sample order's status page, and a `curl` command that orders like the guest cart does.

| Variable | Default | |
|---|---|---|
| `DEMO_PORT` | `8080` | first port to try; the next free one is used |
| `DEMO_HOST` | this machine's LAN address | the address in QR codes; set it if phones cannot reach the default (VPN, a VM's internal network) |
| `DEMO_DIR` | `./.demo` | data directory, wiped on every run |

The demo listens on all network interfaces so phones on the same network can order. Run it only on networks you trust. Its passwords are printed on the screen. `make run` is reachable the same way, on port 8080.
