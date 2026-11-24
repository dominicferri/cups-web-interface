# CUPS webui

**Web interface for CUPS**: a single-binary replacement for the CUPS web interface (`localhost:631`) that macOS 27 removed.

![The CUPS webui dashboard in mock mode, showing the summary strip and the printers and queues table](./docs/screenshot.png)

This copy is an unofficial packaging of [PaperCutSoftware/cups-web-interface](https://github.com/PaperCutSoftware/cups-web-interface). It adds a double-click installer, a server that runs only while you use it, and a set of bug, security and speed fixes. It is not published or supported by PaperCut. The PaperCut name and logo are trademarks of PaperCut Software.

**Contents**

- [What it does](#what-it-does)
- [Install](#install)
- [How the server runs](#how-the-server-runs)
- [Commands and flags](#commands-and-flags)
- [Printer drivers](#printer-drivers)
- [Setting printer accessories manually](#setting-printer-accessories-manually)
- [What differs from upstream](#what-differs-from-upstream)
- [Building from source](#building-from-source)
- [Architecture](#architecture)
- [Project map](#project-map)
- [Security](#security)
- [License](#license)

---

## What it does

- **Printers and queues**: add, modify, delete, pause, resume, set the default, purge a queue, print a test page, and turn sharing on or off.
- **Jobs**: list active and completed jobs; cancel, hold, release and restart them. Stuck jobs get a banner with a one-click cancel.
- **Drivers**: pick a driver from the installed list, swap the driver of an existing queue, or upload your own PPD. See [Printer drivers](#printer-drivers).
- **Printer options and accessories**: change a queue's default options, and set trays, staplers and other installable hardware by hand. See [Setting printer accessories manually](#setting-printer-accessories-manually).
- **Diagnostics**: a log page that marks known problems in the CUPS `error_log`, and an audit that finds queues whose driver filters are Intel-only and need Rosetta.
- **Server settings**: the `cupsctl` settings, plus install and removal of the service, from the page.
- **Live updates**: the page follows changes through an event stream.
- **No setup in CUPS**: it works without `cupsctl WebInterface=yes`, which macOS 27 no longer accepts.
- **Local only**: it binds to `127.0.0.1:6310` and signs in only the macOS user who runs it. See [SECURITY.md](./SECURITY.md).

The page follows the light or dark appearance of your Mac.

---

## Install

A universal binary (Apple Silicon and Intel) is included at [`bin/cups-webui`](./bin/cups-webui). You do not need Go or Node.js.

### Double-click installer

1. Download the folder and keep its contents together.
2. Double-click `Install CUPS WebUI.command`.
3. macOS blocks the file the first time, because it came from the internet and is not notarized. Open **System Settings > Privacy & Security**, scroll to **Security**, and click **Open Anyway**. Then double-click the file again.
4. The installer clears the download flag on `bin/cups-webui`, installs the service, and opens `http://127.0.0.1:6310`.

To skip step 3, clear the flag in Terminal before the first run:
```bash
xattr -dr com.apple.quarantine /path/to/the/folder
```

The installer stops with a message when another program already uses `127.0.0.1:6310`. The file shows the PaperCut icon after its first run. Git and zip downloads do not keep Finder icons, so the installer puts the icon back from `assets/papercut-logo.png`.

### From Terminal

```bash
# Install the service: it starts now
./bin/cups-webui launchagent install

# Or install it on a different port
./bin/cups-webui launchagent install -port 8631

# Check service status
./bin/cups-webui launchagent status

# Open the web interface (starts the server if it is stopped)
./bin/cups-webui open

# Remove the service
./bin/cups-webui launchagent uninstall
```

The install command copies the binary to `~/Library/Application Support/cups-webui/` and starts the service straight away. The service is installed for your user account only. **Install Service** under **CUPS Settings** in the web interface does the same thing, on the port that instance runs on.

If you installed an earlier version, run `launchagent install` again. The old plist made macOS start the server at login and restart it after every exit.

### Without installing

```bash
# Start the server and open the browser
./bin/cups-webui -open

# The same with simulated printers, queues and alerts (no printer needed)
./bin/cups-webui -mock -open
```

The server runs until its browser tab closes or you close the Terminal window. This does not install the service.

---

## How the server runs

The server runs only while you use it.

| Event | What happens |
| :--- | :--- |
| `./bin/cups-webui open` | The command starts the installed service if it is stopped, waits for it, and opens the browser with a sign-in link. |
| You close the last CUPS WebUI tab | The server exits about 15 seconds later. A page reload or a second tab does not stop it. |
| The server started less than 2 minutes ago | It keeps running until the 2 minutes have passed, so a browser that is still opening has time to connect. |
| No browser connects | The server exits 2 minutes after its start. |
| You log in or the Mac boots | Nothing. The service does not start at login. |
| The server exits or crashes | macOS does not restart it. `open` starts it again. |

- To keep a hand-started server running after the browser closes, add `-keep-running`.
- Once the server runs, you can open `http://127.0.0.1:6310` directly. No login step is needed for the account that runs the server. Other accounts on the same Mac are not signed in.
- A bookmark works only while the server runs. Use `./bin/cups-webui open` to get back in.
- While the server is stopped, nothing holds port 6310. On a Mac with more than one account, another user's process can take the port. `open` checks who owns the listener and refuses to open the browser when it is not your account. A bookmark has no such check. See [SECURITY.md](./SECURITY.md).

The server decides that a browser is in use from two things: an open event stream, and API requests that passed the origin and sign-in checks. Rejected requests do not count, so another web page or another local user cannot keep the server running.

---

## Commands and flags

| Command | What it does |
| :--- | :--- |
| `cups-webui [serve] [flags]` | Start the server (the default) |
| `cups-webui open [-port <int>]` | Open the server in your default browser with its session link. Starts the installed service first if it is stopped. Without a live session it opens `http://127.0.0.1:<port>` |
| `cups-webui launchagent install [-port <int>]` | Register the service (default port 6310) |
| `cups-webui launchagent uninstall` | Unload and remove the service |
| `cups-webui launchagent status` | Show whether the service is installed and running |

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port` | `6310` | Port to listen on |
| `-host` | `127.0.0.1` | Host interface to bind to. Only loopback addresses (`127.0.0.1`, `::1`, `localhost`) are accepted; any other value stops the server at start |
| `-mock` | `false` | Run with simulated CUPS mock data. `CUPS_WEBUI_MOCK=1` does the same |
| `-open` | `false` | Open the default browser at start |
| `-keep-running` | `false` | Keep the server running after the browser closes |

Files the program writes:

| Path | Content |
| :--- | :--- |
| `~/.cups-webui/session.json` | Address, process ID and session token of the running server. Readable only by your account. Removed at a clean exit |
| `~/.cups-webui/ppd-uploads/` | Uploaded PPD files, kept for one hour |
| `~/.cups-webui/accessory-overrides.json` | Notes on accessories that were set by hand |
| `~/Library/LaunchAgents/com.papercut.cups-webui.plist` | The service definition |
| `~/Library/Application Support/cups-webui/cups-webui` | The binary the service runs |
| `~/Library/Logs/com.papercut.cups-webui.stdout.log`, `.stderr.log` | Service logs |

---

## Printer drivers

### Driver lists

The Swap Driver and Add Printer lists show first the drivers whose make is in the queue's name or description. A queue such as `Office_Xerox` starts with the Xerox drivers. "Generic" and "Raw" never count as a match. Other queues keep the order `lpinfo` gives.

### Custom PPD uploads

1. The page sends the file to `POST /api/v1/ppds/validate`.
2. The server runs `cupstestppd` and its own scan for dangerous directives. The file must pass both. The page shows the reasons `cupstestppd` gives for a failure.
3. A file that passes is stored in `~/.cups-webui/ppd-uploads/` under a random 32-character ID. The folder is readable only by your account. The response holds that ID as `upload_id`.
4. Add Printer and the driver swap send the ID as `ppd_upload_id`. The server finds the stored file and passes it to `lpadmin -P`.

The page never sends a file path, so the list of allowed PPD folders is unchanged. A stored upload is removed after one hour, the next time a file is uploaded.

### Xerox drivers (optional)

`drivers/` holds the Xerox macOS driver image, named as Xerox ships it (`XeroxDrivers_<version>_<build>.dmg`). After the web interface opens, the installer compares that version with the installed one and offers to install it only when the bundled version is newer. When `drivers/` holds more than one image, the installer uses the one with the highest version. The install asks for an administrator password. The image is Xerox software under the Xerox license, signed and notarized by Xerox Corporation. To update it, replace the file with a newer download from the Xerox support page for your printer model.

---

## Setting printer accessories manually

A driver usually detects a printer's accessories (trays, stapler, hole punch, duplexer) only when the queue is added, and some drivers ask the printer over SNMP. When the queue points at an IPP server such as PaperCut Mobility Print, the Mac can't reach the printer, so those accessories stay "not installed" and features like stapling are unavailable.

The **Hardware Accessories** tab lists the queue's installable options, installed ones first, and marks values that differ from the driver's default as *Detected* or *Set manually*. CUPS administrators can click **Edit accessories** (Advanced) to change them. Saving does the same as `lpadmin -p <queue> -o <Option>=<Choice>`: it updates only that queue's PPD, never the shared driver PPD. The app then reads the PPD back from CUPS and shows what was stored.

- CUPS can't check these settings against the printer. Confirm the printer has the hardware first, for example with the SNMP check below. The warning is stronger when the queue doesn't point straight at the printer.
- The value itself is stored by CUPS in the queue's PPD. Who set it, when, and the value it replaced are notes kept by this app in `~/.cups-webui/accessory-overrides.json`. **Reset to detected values** puts the old values back.
- The notes belong to the macOS account running the service and stay on this Mac: another admin account, or a copy of the queue on another Mac, doesn't see them. "Set by" is that account's name, and the file can be edited by it, so treat the notes as a reminder, not an audit log. For a record of who changed a queue, see CUPS's own log (`/var/log/cups/access_log`).
- Notes are tied to the queue's CUPS identity (`printer-uuid`), so a queue deleted and re-created with the same name doesn't inherit them. Notes for deleted queues are removed. If the notes file is damaged or was written by a newer version of the app, the tab still shows the values from CUPS, but changes are refused until the file is fixed.
- If the queue's driver is replaced from the app, manual values are applied again.
- For drivers in a small built-in table (currently "Generic Universal Mono"), **Ask the device for suggestions** reads the device's IPP attributes and fills in the form. Nothing is saved until you confirm.
- **Check installed hardware (SNMP)** is optional and off by default. A CUPS administrator turns it on under **CUPS Settings > SNMP hardware check**; the setting is saved in `~/.cups-webui/features.json`. In Edit accessories, type the printer's own IP address or host name (not the print server's) and the SNMP community (default `public`). The app runs `snmpwalk` (SNMPv2c, UDP 161) and lists the paper sources, output bins, finishers, duplex unit and devices the printer reports. For drivers in the suggestion table it also offers values to fill in the form; for other drivers, use the list to set the accessories by hand. It works only on a network that can reach the printers themselves, such as an IT network profile. See [SECURITY.md](./SECURITY.md) for how the address and community are handled.
- The section isn't shown for driverless (AirPrint / IPP Everywhere) queues or for drivers without installable options.

### Print server list (CSV)

When queues point at a Windows print server, the Mac doesn't know each printer's own address. Import the print server's printer list under **CUPS Settings > SNMP hardware check > Print server list (CSV)**, and the SNMP form fills in the address of each queue's printer. An administrator imports it; a new import replaces the old list, and **Remove list** forgets it.

Export the list on the print server with PowerShell, for example:

```powershell
Get-Printer | ForEach-Object {
    $port = Get-PrinterPort -Name $_.PortName -ErrorAction SilentlyContinue
    [pscustomobject]@{
        PrinterName = $_.Name
        Model       = ''   # optional; fill it in if you have it
        Driver      = $_.DriverName
        PortName    = $_.PortName
        IPAddress   = if ($port.PrinterHostAddress) { $port.PrinterHostAddress } else { 'N/A' }
    }
} | Export-Csv PrinterInventory.csv -NoTypeInformation -Encoding UTF8
```

The file format:

| Column | Required | Use |
|---|---|---|
| `PrinterName` | Yes | The printer's name on the print server. |
| `IPAddress` | `IPAddress` or `PortName` | The printer's IP address or host name. `N/A` or empty means "use `PortName`". |
| `PortName` | `IPAddress` or `PortName` | The Windows port. An address is taken from `10.0.0.5`, `IP_10.0.0.5` or `10.0.0.5_1`. Local ports such as `TS001`, `PORTPROMPT:` and `nul` have no address. |
| `Model` | No | Shown next to the address. `SNMP unavailable` is ignored. |
| `Driver` | No | Shown next to the address. |

Example (the addresses are documentation addresses):

```csv
"PrinterName","Model","Driver","PortName","IPAddress"
"Front Desk (Main)","Example Laser 400","Example PCL6","192.0.2.10","192.0.2.10"
"Library 2.01","SNMP unavailable","Example PS","IP_192.0.2.11","N/A"
"Kiosk","","Example PCL6","TS001","N/A"
```

- Column names are matched without regard to case or order; other columns are ignored. The file can be UTF-8 (with or without a byte order mark) or UTF-16 with a byte order mark, and may start with the `#TYPE` line that Windows PowerShell 5 writes without `-NoTypeInformation`. The limit is 2 MB and 5,000 printers.
- A queue finds its printer by the printer name in its device address (`ipps://server:9164/printers/Front%20Desk%20%28Main%29`), or else by the queue name macOS makes from that name (`Front_Desk__Main_`). A queue with a direct device address keeps that address.
- Rows that can't be used are skipped and listed with their line number and the reason: no address, an `IPAddress` that isn't an address, a wrong number of fields, or a name already used on an earlier line (the first one is kept). The result also says how many queues on this Mac found their printer.
- The import is refused, and the old list is kept, when the file is empty, isn't text, isn't valid CSV (the message names the line), has no `PrinterName` column or no `IPAddress`/`PortName` column, has a column twice, has no printers, or has no printer with a usable address.
- Only the parsed list is kept, in `~/.cups-webui/printer-inventory.json` (readable only by your account). The CSV file itself is not stored. The list maps your printers to internal addresses: keep the export out of repositories and shared folders.

---

## What differs from upstream

[CHANGELOG.md](./CHANGELOG.md) has the dated list. In short:

- **Installer**: `Install CUPS WebUI.command`, with the optional Xerox driver install.
- **On-demand server**: the server stops when the browser closes, and the service no longer starts at login. See [How the server runs](#how-the-server-runs).
- **19 defects from a first code review**: 12 functional bugs and 7 security weaknesses, in the two tables below.
- **7 findings from a review of the on-demand server**, in the third table.
- **Speed** and **driver list order**.
- **Console error**: the page no longer logs a Content-Security-Policy error on each load. The inline theme script is now a file (`theme-init.js`) that `script-src 'self'` allows. The policy is unchanged.

### Functional fixes

| Area | Before | Now |
| :--- | :--- | :--- |
| Printer sharing | Saving queue details turned sharing off. The server never read the sharing flag from CUPS, so the form started with sharing off and sent that value back. | The server reads `printer-is-shared` for every queue with one `ipptool` request. The form sends the flag only when you change it. |
| Custom PPD upload | Add Printer and the driver swap validated an uploaded PPD and then dropped it. The queue was created with no driver, and the driver swap sent only the file name. | A valid upload is stored and installed with `lpadmin -P`. See [Custom PPD uploads](#custom-ppd-uploads). |
| Job age | `lpstat` prints local time, and the server read it as UTC. West of UTC every active job looked hours old and was marked as stuck. | Job times are read as local time. |
| Held jobs | Every active job was "pending", and the first job of each queue was forced to "processing". A held job never showed as held, so **Release** never appeared. | Job states come from CUPS (`job-state`). The old guess is used only when `ipptool` gives no answer. A held job is not counted as stuck. |
| Live updates | The server's 15-second write timeout closed the event stream every 15 seconds. The 5-second poll hid the problem. | The event stream has no write deadline and stays connected. |
| Job list errors | A failed `lpstat` call showed as an empty job list. | The request returns an error, and the page shows it. |
| PPD validation | The page said "invalid" with no reason, because the server looked for `FAIL:` and `WARN:` and `cupstestppd` prints `**FAIL**` and `WARN`. | The reasons from `cupstestppd` are shown. |
| Test page message | The message showed `Job #[object Object]`. | It shows the job number. |
| Messages | A new message could disappear after half a second, because the timer of the previous message cleared it. | Each new message cancels the earlier timer. |
| Service reinstall | The installer stopped the old service and deleted its binary before the new one was in place. A failed copy left no working service. Install and uninstall from the web page also stopped the process that was handling the request. | The new binary is staged first and replaces the old one in one rename. From the web page, a separate helper process does the work. |
| LaunchAgent plist | A path with `&` or `<` in it gave an invalid plist. | Every value is XML-escaped. |
| `cups-webui open` | It exited with status 0 when the browser could not be opened, so the installer could not report that. | It exits with status 1 and prints the error. |
| Installer, driver image | With two images, `5.9.0` was chosen over `5.20.0`. Newer installed drivers got an offer to downgrade. An image with no package ran `sudo installer` with an empty path. | Versions are compared as versions. The offer appears only for a newer bundled version. An image with no package stops with a clear message. |

### Security fixes

| Area | Before | Now |
| :--- | :--- | :--- |
| Session token in the log | At start the server printed the sign-in link, which holds the token. Under launchd that went to `~/Library/Logs/com.papercut.cups-webui.stdout.log`. | The link is printed only when the output is a terminal. |
| Loopback only | `-host 0.0.0.0` was accepted and put the admin API on the network. `Host: 0.0.0.0` was an accepted header. `-host ::1` failed to bind. | The server stops at start unless `-host` is `127.0.0.1`, `::1` or `localhost`. `0.0.0.0` is refused as a `Host` header. `::1` works. |
| Jobs request | The `printer` value of `GET /api/v1/jobs` went to `lpstat` unchecked. A value such as `-hother.host` was read as an option. | `printer` must be a valid queue name and `which` must be `active`, `completed` or `all`. Anything else returns 400. |
| Passwords in errors | A failed command returned its full argument list, so `smb://user:password@server/queue` showed the password on the page. | The user name and password of any address in an error message are replaced with `<redacted>`. |
| PPD safety scan | The scan for `*FoomaticRIPCommandLine` and untrusted filter paths stopped without an error at the first line longer than 64 KB. A file with CR-only line endings was read as one line. | The scan reads lines of any length and both line endings. |
| Token lifetime | After a crash the next start reused the token left in `~/.cups-webui/session.json`. | Every start generates a new token. An open page signs in again by itself. |
| Server settings | A settings request that left a field out turned that setting off. | A field that is not sent keeps its current value. |

### On-demand server review

The first version of the on-demand server went through a second review. These are its 7 findings.

| Area | First version | Now |
| :--- | :--- | :--- |
| Start at login | The plist used `KeepAlive` with `SuccessfulExit` false. That key implies `RunAtLoad`, so the server still started at every login. A server that could not bind its port was restarted about every 10 seconds. | The plist has no `KeepAlive`. The server does not start at login and is not restarted. |
| Free port | With the server stopped, another account on the Mac could bind the port, and `open` would show that process's page. | `open` refuses to open the browser when the listener belongs to another account. A bookmark is still not checked. |
| `open` errors | `open` exited with status 1 when the service did not start or no session file appeared, even with a server running. | It prints a warning and opens the default address. It skips the start when macOS already runs the service. |
| Server test | `open` tested the server with a bare TCP connection, which did not reset the idle timer. The server could stop before the browser arrived. | `open` sends an API request, which resets the timer. A request also no longer ends the 2-minute start window. |
| Idle timer reset | Any API request reset the timer, including one the server then rejected. A web page or another local user could keep the server running. | Only a request that passed the origin and sign-in checks counts. |
| Closing event stream | The stream left the subscriber count before it recorded its closing time. The server could stop with no grace period. | The time is recorded first. |
| Clock | The timer used the wall clock. A time sync or a manual change could delay the shutdown or end the grace period at once. | The timer uses the monotonic clock. |

### Speed

Measured on a Mac with 5 queues and a 614 KB `error_log`.

- The log page loads in about 7 ms, down from about 280 ms. The server reads the log from the newest line back and stops at the number of lines asked for.
- One 5-second refresh of the page reads the printer list once, not twice. The list is shared for 1 second. The status request takes about 255 ms, down from about 400 ms.
- The two slow `lpstat` calls run at the same time. A printer list read with nothing stored takes about 200 ms, down from about 290 ms.
- Every command that changes CUPS drops the shared list, so the page shows the change on its next read. A change made outside CUPS WebUI can show up to 1 second later than before.

### How the fixes were checked

- Unit tests cover each fix. They are in `internal/api/review_fixes_test.go`, `internal/cups/review_fixes_test.go`, `internal/api/idle_test.go`, `internal/macos/launchagent_test.go`, `internal/peercred/peercred_darwin_test.go`, `internal/web/embed_test.go` and `cmd/cups-webui/main_test.go`.
- The upstream binary and the new binary were run side by side against the same CUPS server. The upstream binary showed each of these faults and the new binary did not: the event stream cut at 15 seconds, the unchecked `printer` value, the empty job list for a failed `lpstat`, `-host 0.0.0.0`, `-host ::1`, the token in redirected output, the missing PPD failure reason, the dropped PPD upload, and the exit status of `open`.
- A browser run in mock mode covered Add Printer with an uploaded PPD, the driver swap with an uploaded PPD, both message fixes, and sharing staying on after a save.
- `open` was run from the built binary in a separate home folder with a stand-in browser launcher. It opened the session link for a live server, opened the plain address with no session file and with the server stopped, and refused the root-owned CUPS listener on port 631 with status 1.
- The service was reinstalled from the built binary on one Mac. The installed plist has no `KeepAlive`, and the server stayed up while a browser tab was open.

These parts have tests but were not run against a real system:

- Creating a real queue from an uploaded PPD with `lpadmin -P`.
- Reinstall and uninstall from the web page while the server runs as the service.
- The driver package install in the installer.
- Sharing, held jobs and stuck jobs on a CUPS server that has a shared queue or an active job.
- A login with the new plist installed, to confirm that the server stays stopped.
- `open` starting a stopped service through macOS.
- A real listener of a second user account on the server's port. The check was run against a root-owned listener.

---

## Building from source

### Prerequisites
- [Go](https://go.dev/) (1.26+)
- [Node.js](https://nodejs.org/) (20+) and `npm`

The Go code uses only the standard library, so `go.mod` lists no dependencies and there is no `go.sum` file.

### Build steps
```bash
# Build the frontend SPA and compile the universal macOS binary (bin/cups-webui):
make build

# Or compile only the native Go binary for your current machine architecture:
make build-go

# Or build the frontend independently:
cd web && npm install && npm run build
```

### Running tests
Run the Go test suite:
```bash
make test
```

**`make test` changes the CUPS settings of the Mac it runs on.** Three tests call the real `lpadmin` and `lpoptions` commands:

- `TestCLIAdapterSetDefaultPrinter` makes the first queue (in name order) the system default printer and writes `~/.cups/lpoptions`.
- `TestCLIAdapterSetPrinterOptions` sets the first option of that queue to its first choice.
- `TestCLIAdapterAddAndRemoveKyoceraPrinter` adds and deletes a test queue when a Kyocera driver is installed.

To run every other test and leave CUPS alone:
```bash
go test ./... -skip 'TestCLIAdapter(AddAndRemoveKyoceraPrinter|SetDefaultPrinter|SetPrinterOptions)'
```

---

## Architecture

- **Backend (`cmd/cups-webui/main.go`, `internal/api/`, `internal/cups/`)**: Pure Go, standard library only, using `net/http` and the native CUPS command-line tools (`lpstat`, `lpadmin`, `lpoptions`, `cancel`, `cupsctl`, `cupsdisable`, `cupsenable`).
- **Platform code (`internal/macos/`, `internal/peercred/`)**: the LaunchAgent service, and the lookup of which user owns a loopback connection.
- **Frontend (`web/`)**: React 19 + TypeScript + Tailwind CSS v4 SPA, compiled directly into `internal/web/dist` and embedded into the Go binary with `go:embed`.
- **Packaging**: Single executable (`./bin/cups-webui`) with zero runtime dependencies.

---

<!-- git-understand:start -->
## Project map

This repository is the PaperCut CUPS WebUI with a double-click installer added for macOS 27, which removed the built-in CUPS web interface. The Go backend is 13,845 lines and the React frontend is 6,689 lines.

The two diagrams come from a code graph built on 2026-11-19, 15 commits before commit `8f33d92`. The Composition and History tables were measured at commit `8f33d92`.

- The CUPS Adapter Layer is the base of the project. It holds 278 of the 481 nodes in the graph, and the HTTP API Layer has 142 links into it, which is three times the next largest arrow.
- 'IsValidPrinterName' in 'internal/cups/validation.go' is called 34 times, more than twice the next function.
- The frontend and the Go code do not import each other. In the full graph the only link between them is 'web/vite.config.ts', which configures 'internal/web/embed.go'. At run time the frontend calls the Go API over HTTP from 'web/src/api/client.ts', and the graph does not draw that.
- 'cmd/cups-webui/main.go' has 36 outgoing links, the most of any file.

### Architecture

The arrow to look at goes from the HTTP API Layer to the CUPS Adapter Layer: 114 imports and 28 calls. The CUPS Adapter Layer has only 6 links out, 5 to macOS Platform Services and 1 test link to the HTTP API Layer.

The 8 strongest of 19 links between layers.

```mermaid
flowchart TD
  L0["HTTP API Layer<br/>10 files"]
  L1["Application Entry Point<br/>2 files"]
  L2["Frontend UI<br/>17 files"]
  L3["Build, Install and Configuration<br/>10 files"]
  L4["Documentation<br/>4 files"]
  L5["macOS Platform Services<br/>6 files"]
  L6["Frontend API Client and Shared Code<br/>5 files"]
  L7["CUPS Adapter Layer<br/>19 files"]
  L0 -->|142| L7
  L2 -->|46| L6
  L1 -->|21| L7
  L1 -->|13| L0
  L1 -->|11| L5
  L3 -->|7| L2
  L0 -->|5| L5
  L3 -->|5| L1
```

### Dependencies

'web/src/types/cups.ts' has the highest fan-in at 15, and on the Go side 'internal/cups/validation.go' has 14.

9 of 71 files: the most connected ones, test files left out, joined by the 10 strongest of the 29 links between them.

```mermaid
flowchart LR
  subgraph G0["HTTP API Layer"]
    direction TB
    N2["api.go"]
    N5["accessories.go"]
    N6["handlers.go"]
  end
  N0["main.go"]
  N12["App.tsx"]
  N13["cups.ts"]
  subgraph G7["CUPS Adapter Layer"]
    direction TB
    N7["cli_adapter.go"]
    N10["accessories.go"]
    N11["validation.go"]
  end
  N6 --> N11
  N7 --> N11
  N10 --> N11
  N5 --> N11
  N0 --> N2
  N0 --> N7
  N7 --> N10
  N12 --> N13
  N0 --> N11
  N2 --> N11
```

### Composition

Go is 13,845 lines in 48 files and TypeScript is 6,689 lines in 20 files.

117 files, 23626 lines.

| Language | Files | Lines |
|---|---:|---:|
| Go | 48 | 13845 |
| TypeScript | 20 | 6689 |
| Lockfile | 1 | 1982 |
| Docs | 4 | 485 |
| CSS | 3 | 225 |
| Other | 29 | 133 |
| JSON | 5 | 95 |
| Shell | 1 | 71 |

### History

The Go and TypeScript source arrived in the first commit. The first commits after it changed the installer and the documents. The later ones changed the source: the review fixes, the speed work, the driver list order and the on-demand server.

19 commits, 2026-11-19 to 2026-11-20.

| Hotspot | Commits | Lines |
|---|---:|---:|
| `internal/cups/cli_adapter.go` | 3 | 1514 |
| `web/src/components/PrinterConfigModal.tsx` | 3 | 967 |
| `internal/cups/mock_adapter.go` | 2 | 1428 |
| `web/src/components/AddPrinterModal.tsx` | 3 | 929 |
| `cmd/cups-webui/main.go` | 4 | 430 |
| `internal/macos/launchagent.go` | 4 | 327 |
| `web/src/App.tsx` | 2 | 635 |
| `internal/api/handlers.go` | 2 | 623 |

| Changed together | Commits | Linked in code |
|---|---:|---|
| `cmd/cups-webui/main.go` + `internal/api/events.go` | 3 | yes |
| `cmd/cups-webui/main.go` + `internal/api/security.go` | 3 | yes |
| `cmd/cups-webui/main.go` + `internal/macos/launchagent.go` | 3 | yes |
| `cmd/cups-webui/main.go` + `internal/macos/launchagent_test.go` | 3 | yes |

<!-- git-understand:end -->

---

## Security

The security model is described in [SECURITY.md](./SECURITY.md). Please report suspected vulnerabilities in the upstream project through <https://www.papercut.com/contact/security/> and not as public issues.

## License

MIT. See [LICENSE](./LICENSE). Bundled fonts are covered by their own licences; see [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

## About the upstream project

CUPS WebUI (`cups-webui`) is an open-source project maintained by [PaperCut Software](https://www.papercut.com/), makers of print management software. It was built after Apple decided to remove the built-in CUPS Web Interface (`localhost:631`) in macOS 27 Golden Gate. Although admins can still use commands like `lpadmin` and `lpstat` to manage print queues and default settings, `cups-webui` offers a much easier way to manage them.
