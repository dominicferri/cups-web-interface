# CUPS webui

**Web interface for CUPS**: a modern, lightweight, single-binary replacement for the legacy CUPS web interface (`localhost:631`) on macOS.

![The CUPS webui dashboard in mock mode, showing the summary strip and the printers and queues table](./docs/screenshot.png)

Designed specifically to address UX barriers in macOS and UNIX environments:
- **Zero Configuration Access**: Bypasses the requirement to run `cupsctl WebInterface=yes` in Terminal.
- **Human-Centric UX**: Replaces technical abstractions (*PPDs*, *Classes*) with clear workflows (*Printers*, *Print Queues*, *Printer Pools*).
- **Modern Responsive Interface**: Clean dashboard that follows your Mac's light or dark appearance, built with Tailwind CSS and Lucide icons.
- **Queue Diagnostics & Stuck Job Remediation**: Identifies stuck print jobs and provides instant one-click cancellation.
- **Security-First**: Binds to loopback `127.0.0.1:6310` by default, and signs in only the macOS user who runs it. See [SECURITY.md](./SECURITY.md).

---

## One-Click Install (unofficial packaging)

This copy adds `Install CUPS WebUI.command` and a set of bug and security fixes (see [CHANGELOG.md](./CHANGELOG.md)) to the upstream project, [PaperCutSoftware/cups-web-interface](https://github.com/PaperCutSoftware/cups-web-interface). It is not published or supported by PaperCut. The PaperCut name and logo are trademarks of PaperCut Software.

1. Download the folder and keep its contents together.
2. Double-click `Install CUPS WebUI.command`.
3. macOS blocks the file the first time, because it came from the internet and is not notarized. Open **System Settings > Privacy & Security**, scroll to **Security**, and click **Open Anyway**. Then double-click the file again.
4. The installer clears the download flag on `bin/cups-webui`, installs the login service, and opens `http://127.0.0.1:6310`.

To skip step 3, clear the flag in Terminal before the first run:
```bash
xattr -dr com.apple.quarantine /path/to/the/folder
```

The file shows the PaperCut icon after its first run. Git and zip downloads do not keep Finder icons, so the installer puts the icon back from `assets/papercut-logo.png`.

To remove the service: `./bin/cups-webui launchagent uninstall`

### Xerox drivers (optional)

`drivers/` holds the Xerox macOS driver image, named as Xerox ships it (`XeroxDrivers_<version>_<build>.dmg`). After the web interface opens, the installer compares that version with the installed one and offers to install it only when the bundled version is newer. When `drivers/` holds more than one image, the installer uses the one with the highest version. The install asks for an administrator password. The image is Xerox software under the Xerox license, signed and notarized by Xerox Corporation. To update it, replace the file with a newer download from the Xerox support page for your printer model.

---

## Fixes in This Copy

This copy differs from the upstream source. A code review found 19 defects, and all 19 are fixed here: 12 functional bugs and 7 security weaknesses. [CHANGELOG.md](./CHANGELOG.md) has the dated list.

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

### Custom PPD uploads

1. The page sends the file to `POST /api/v1/ppds/validate`.
2. The server runs `cupstestppd` and its own scan for dangerous directives. The file must pass both.
3. A file that passes is stored in `~/.cups-webui/ppd-uploads/` under a random 32-character ID. The folder is readable only by your account. The response holds that ID as `upload_id`.
4. Add Printer and the driver swap send the ID as `ppd_upload_id`. The server finds the stored file and passes it to `lpadmin -P`.

The page never sends a file path, so the list of allowed PPD folders is unchanged. A stored upload is removed after one hour, the next time a file is uploaded.

### How the fixes were checked

- Unit tests cover each fix. They are in `internal/api/review_fixes_test.go`, `internal/cups/review_fixes_test.go` and `internal/macos/launchagent_test.go`.
- The upstream binary and the new binary were run side by side against the same CUPS server. The upstream binary showed each of these faults and the new binary did not: the event stream cut at 15 seconds, the unchecked `printer` value, the empty job list for a failed `lpstat`, `-host 0.0.0.0`, `-host ::1`, the token in redirected output, the missing PPD failure reason, the dropped PPD upload, and the exit status of `open`.
- A browser run in mock mode covered Add Printer with an uploaded PPD, the driver swap with an uploaded PPD, both message fixes, and sharing staying on after a save.

These parts have tests but were not run against a real system:

- Creating a real queue from an uploaded PPD with `lpadmin -P`.
- Reinstall and uninstall from the web page while the server runs as the login service.
- The driver package install in the installer.
- Sharing, held jobs and stuck jobs on a CUPS server that has a shared queue or an active job.

### Known issue

`web/index.html` has an inline script that sets the theme before the page loads. The server's Content-Security-Policy (`script-src 'self'`) blocks it, and the browser console shows one error on each load. The page still works. This is the same in the upstream source and is not fixed here.

---

## Quick Start

A pre-compiled universal binary for macOS (supporting both Apple Silicon and Intel) is included directly in [`bin/cups-webui`](./bin/cups-webui), so you can run the app immediately without installing Go or Node.js.

### 1. Launch the Web Interface
Start the server and automatically open the interface in your browser:
```bash
./bin/cups-webui -open
```
The interface will be available at `http://127.0.0.1:6310`. Once the server is running, you can open (or bookmark) that address directly; no login step is needed for the user account that runs the server. Other accounts on the same Mac are not signed in.

This runs until you close the Terminal window. To keep it running and start it at every login, see step 2.

### 2. Start Automatically at Login (Recommended)
Running the install command is all you need. It sets up cups-webui as a background service (a macOS LaunchAgent) that starts every time you log in and restarts if it stops. You don't need to enable anything else in the web interface.
```bash
# Install the service: it starts now and at every login
./bin/cups-webui launchagent install

# Or install it on a different port
./bin/cups-webui launchagent install -port 8631

# Check service status
./bin/cups-webui launchagent status

# Remove the service (it stops and no longer starts at login)
./bin/cups-webui launchagent uninstall
```

The install command copies the binary to `~/Library/Application Support/cups-webui/` and starts the service straight away. Open `http://127.0.0.1:6310` in your browser and bookmark it (or the port you chose with `-port`).

**Enable Auto-Start** under **CUPS Settings** in the web interface does the same thing, so use whichever is easier. It uses the port that instance is running on. Running `./bin/cups-webui -open` on its own (step 1) does not start it at login.

The service starts when you log in, not when the Mac boots, and only for your user account.

### 3. Run in Mock Mode (For Testing / Demo)
Runs with simulated printers, queues, and diagnostic alerts without requiring active printer hardware:
```bash
./bin/cups-webui -mock -open
```

---

## Setting Printer Accessories Manually

A driver usually detects a printer's accessories (trays, stapler, hole punch, duplexer) only when the queue is added, and some drivers ask the printer over SNMP. When the queue points at an IPP server such as PaperCut Mobility Print, the Mac can't reach the printer, so those accessories stay "not installed" and features like stapling are unavailable.

The **Hardware Accessories** tab lists the queue's installable options, installed ones first, and marks values that differ from the driver's default as *Detected* or *Set manually*. CUPS administrators can click **Edit accessories** (Advanced) to change them. Saving does the same as `lpadmin -p <queue> -o <Option>=<Choice>`: it updates only that queue's PPD, never the shared driver PPD. The app then reads the PPD back from CUPS and shows what was stored.

- These settings can't be checked against the printer. Confirm the printer has the hardware first. The warning is stronger when the queue doesn't point straight at the printer.
- The value itself is stored by CUPS in the queue's PPD. Who set it, when, and the value it replaced are notes kept by this app in `~/.cups-webui/accessory-overrides.json`. **Reset to detected values** puts the old values back.
- The notes belong to the macOS account running the service and stay on this Mac: another admin account, or a copy of the queue on another Mac, doesn't see them. "Set by" is that account's name, and the file can be edited by it, so treat the notes as a reminder, not an audit log. For a record of who changed a queue, see CUPS's own log (`/var/log/cups/access_log`).
- Notes are tied to the queue's CUPS identity (`printer-uuid`), so a queue deleted and re-created with the same name doesn't inherit them. Notes for deleted queues are removed. If the notes file is damaged or was written by a newer version of the app, the tab still shows the values from CUPS, but changes are refused until the file is fixed.
- If the queue's driver is replaced from the app, manual values are applied again.
- For drivers in a small built-in table (currently "Generic Universal Mono"), **Ask the device for suggestions** reads the device's IPP attributes and fills in the form. Nothing is saved until you confirm.
- The section isn't shown for driverless (AirPrint / IPP Everywhere) queues or for drivers without installable options.

---

## CLI Options

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port` | `6310` | Port to listen on |
| `-host` | `127.0.0.1` | Host interface to bind to. Only loopback addresses (`127.0.0.1`, `::1`, `localhost`) are accepted; any other value stops the server at start |
| `-mock` | `false` | Run with simulated CUPS mock data |
| `-open` | `false` | Automatically open default browser on launch |

Subcommands:
- `cups-webui open` – Open the running server in your default browser using its session link
- `cups-webui launchagent [install|uninstall|status]` – Manage the macOS background LaunchAgent service

---

## Building from Source

If you would like to build or modify the project yourself:

### Prerequisites
- [Go](https://go.dev/) (1.26+)
- [Node.js](https://nodejs.org/) (20+) and `npm`

The Go code uses only the standard library, so `go.mod` lists no dependencies and there is no `go.sum` file.

### Build Steps
```bash
# Build the frontend SPA and compile the universal macOS binary (bin/cups-webui):
make build

# Or compile only the native Go binary for your current machine architecture:
make build-go

# Or build the frontend independently:
cd web && npm install && npm run build
```

### Running Tests
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
- **Frontend (`web/`)**: React 19 + TypeScript + Tailwind CSS v4 SPA, compiled directly into `internal/web/dist` and embedded into the Go binary with `go:embed`.
- **Packaging**: Single executable (`./bin/cups-webui`) with zero runtime dependencies.

---

<!-- git-understand:start -->
## Project map

This repository is the PaperCut CUPS WebUI with a double-click installer added for macOS 27, which removed the built-in CUPS web interface. The Go backend is 12,250 lines and the React frontend is 6,651 lines. Most of the structure is in one place, because the CUPS Adapter Layer holds 278 of the 481 nodes in the graph.

- The CUPS Adapter Layer is the base of the project. The HTTP API Layer has 142 links into it, which is three times the next largest arrow.
- 'IsValidPrinterName' in 'internal/cups/validation.go' is called 34 times, more than twice the next function.
- The frontend and the Go code do not import each other. In the diagram the only link between them is 'web/vite.config.ts', which configures 'internal/web/embed.go'. At run time the frontend calls the Go API over HTTP from 'web/src/api/client.ts', and the graph does not draw that.
- 'cmd/cups-webui/main.go' has 36 outgoing links, the most of any file.
- The Go and TypeScript source arrived in the first commit. The commits after it changed the installer, the documents, '.gitignore' and the driver image, until the fixes listed under "Fixes in This Copy" changed the Go and TypeScript source.

### Architecture

The arrow to look at goes from the HTTP API Layer to the CUPS Adapter Layer: 114 imports and 28 calls. The CUPS Adapter Layer has only 6 links out, 5 to macOS Platform Services and 1 test link to the HTTP API Layer.

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
  L3 -->|5| L7
  L4 -->|5| L3
  L4 -->|5| L7
  L7 -->|5| L5
  L3 -->|4| L0
  L3 -->|3| L5
  L4 -->|3| L0
  L4 -->|3| L1
  L4 -->|2| L2
  L4 -->|1| L5
  L7 -->|1| L0
```

### Dependencies

'web/src/types/cups.ts' has the highest fan-in at 15, and on the Go side 'internal/cups/validation.go' has 14.

The 24 most connected of 71 files.

```mermaid
flowchart LR
  subgraph G0["HTTP API Layer"]
    direction TB
    N1["api_test.go"]
    N2["api.go"]
    N3["peer_test.go"]
    N4["accessories_test.go"]
    N5["accessories.go"]
    N6["handlers.go"]
  end
  N0["main.go"]
  N12["App.tsx"]
  N9["Makefile"]
  N14["README.md"]
  N13["cups.ts"]
  subgraph G7["CUPS Adapter Layer"]
    direction TB
    N7["cli_adapter.go"]
    N8["accessories_test.go"]
    N10["accessories.go"]
    N11["validation.go"]
    N15["accessory_store.go"]
    N16["mock_adapter.go"]
    N17["cli_accessories.go"]
    N18["diagnostics.go"]
    N19["parser_test.go"]
    N20["ppd_installable.go"]
    N21["arch_inspector.go"]
    N22["cli_adapter_test.go"]
    N23["diagnostics_test.go"]
  end
  N0 --> N1
  N0 --> N2
  N0 --> N3
  N0 --> N4
  N0 --> N5
  N0 --> N6
  N0 --> N7
  N0 --> N8
  N0 --> N10
  N0 --> N11
  N0 --> N15
  N0 --> N16
  N0 --> N17
  N0 --> N18
  N0 --> N19
  N0 --> N20
  N0 --> N21
  N0 --> N22
  N0 --> N23
  N1 --> N7
  N1 --> N8
  N1 --> N10
  N1 --> N11
  N1 --> N15
  N1 --> N16
  N1 --> N17
  N1 --> N18
  N1 --> N19
  N1 --> N20
  N1 --> N21
  N1 --> N22
  N1 --> N23
  N2 --> N1
  N2 --> N3
  N2 --> N7
  N2 --> N8
  N2 --> N10
  N2 --> N11
  N2 --> N15
  N2 --> N16
  N2 --> N17
  N2 --> N18
  N2 --> N19
  N2 --> N20
  N2 --> N21
  N2 --> N22
  N2 --> N23
  N3 --> N2
  N3 --> N7
  N3 --> N8
  N3 --> N10
  N3 --> N11
  N3 --> N15
  N3 --> N16
  N3 --> N17
  N3 --> N18
  N3 --> N19
  N3 --> N20
  N3 --> N21
  N3 --> N22
  N3 --> N23
  N4 --> N7
  N4 --> N8
  N4 --> N10
  N4 --> N11
  N4 --> N15
  N4 --> N16
  N4 --> N17
  N4 --> N18
  N4 --> N19
  N4 --> N20
  N4 --> N21
  N4 --> N22
  N4 --> N23
  N5 --> N4
  N5 --> N7
  N5 --> N8
  N5 --> N10
  N5 --> N11
  N5 --> N15
  N5 --> N16
  N5 --> N17
  N5 --> N18
  N5 --> N19
  N5 --> N20
  N5 --> N21
  N5 --> N22
  N5 --> N23
  N6 --> N1
  N6 --> N7
  N6 --> N8
  N6 --> N10
  N6 --> N11
  N6 --> N15
  N6 --> N16
  N6 --> N17
  N6 --> N18
  N6 --> N19
  N6 --> N20
  N6 --> N21
  N6 --> N22
  N6 --> N23
  N7 --> N10
  N7 --> N11
  N7 --> N18
  N7 --> N20
  N7 --> N21
  N7 --> N22
  N8 --> N7
  N8 --> N10
  N8 --> N15
  N8 --> N16
  N8 --> N20
  N9 --> N0
  N9 --> N1
  N9 --> N3
  N9 --> N4
  N9 --> N8
  N9 --> N19
  N9 --> N22
  N9 --> N23
  N10 --> N8
  N10 --> N11
  N10 --> N15
  N10 --> N20
  N12 --> N13
  N14 --> N0
  N14 --> N7
  N14 --> N9
  N14 --> N15
  N14 --> N16
  N15 --> N8
  N16 --> N1
  N16 --> N8
  N16 --> N18
  N17 --> N8
  N17 --> N10
  N17 --> N11
  N17 --> N21
  N18 --> N23
  N19 --> N11
  N20 --> N8
  N21 --> N11
  N22 --> N7
  N23 --> N18
```

### Composition

Go is 12,250 lines in 37 files and TypeScript is 6,651 lines in 20 files. The 'internal' directory is 11,992 lines. That is more than 'web' at 9,034 lines, and 1,982 of the 'web' lines are the lockfile.

104 files, 22004 lines.

| Language | Files | Lines |
|---|---:|---:|
| Go | 37 | 12250 |
| TypeScript | 20 | 6651 |
| Lockfile | 1 | 1982 |
| Docs | 4 | 506 |
| CSS | 3 | 225 |
| Other | 29 | 133 |
| JSON | 5 | 95 |
| Shell | 1 | 63 |

### History

Every commit is from 2026-11-19. The numbers in this section and in the Composition section were measured before the fixes listed under "Fixes in This Copy". Up to that point the changes after the first commit are to the installer and the documents.

7 commits, 2026-11-19.

| Hotspot | Commits | Lines |
|---|---:|---:|
| `Install CUPS WebUI.command` | 3 | 63 |

The table has one row because the installer is the only code file with more than one commit. The Go and TypeScript files have one commit each.

<!-- git-understand:end -->

---

## Security

The security model is described in [SECURITY.md](./SECURITY.md). Please report suspected vulnerabilities through <https://www.papercut.com/contact/security/> and not as public issues.

## License

MIT. See [LICENSE](./LICENSE). Bundled fonts are covered by their own licences; see [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

## About This Project

CUPS WebUI (`cups-webui`) is an open-source project actively maintained by [PaperCut Software](https://www.papercut.com/), makers of print management software. It was built after Apple decided to remove the built-in CUPS Web Interface (`localhost:631`) in macOS 27 Golden Gate. Although admins can still use commands like `lpadmin` and `lpstat` to manage print queues and default settings, `cups-webui` offers a much easier way to manage them.

