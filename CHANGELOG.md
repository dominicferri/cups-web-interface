# Changelog

## 2026-11-23

### Optional SNMP hardware check

- The Hardware Accessories tab can ask a printer over SNMP what hardware it has installed. A queue that points at a print server such as PaperCut can't see the printer, so its trays, finisher and duplex unit stay "not installed". When this Mac is on a network that reaches the printers (for example an IT network profile), an administrator can type the printer's address and see what it reports.
- Off by default. A CUPS administrator turns it on under CUPS Settings > SNMP hardware check. The server checks the setting (`~/.cups-webui/features.json`) on every request; a missing or damaged file keeps it off.
- The check runs `/usr/bin/snmpwalk` (SNMPv2c) on the Printer MIB input, output and media path tables, the Finisher MIB device table and the Host Resources device list, with numeric OIDs checked against RFC 3805 and RFC 3806. It lists paper sources with capacity, output bins, finishers, duplex and devices for any driver.
- For drivers in the suggestion table ("Generic Universal Mono"), it also suggests values: drawers in index order map to the tray options, a manual feed to the bypass tray, stitcher, puncher and folder finishers to the stapler, hole punch and folder options, and a duplex media path to the duplexer. A finisher reported as not present or off suggests nothing. Suggestions only fill in the form.
- The address must be an IP address or host name, with no port or scheme. The community goes in a private temporary `snmp.conf`, never on the command line or in the process list, and is not logged or returned.
- The IPP suggestions and the SNMP check share one suggestion list. The IPP evidence text is unchanged.
- New API: `POST /api/v1/printers/{name}/accessories/snmp`, `GET`/`PUT /api/v1/accessories/features`. The accessories report has `snmp_enabled` and `snmp_host_hint` (the host of a direct device address, used to prefill the form).

### Print server list (CSV import)

- CUPS Settings can import a printer list exported from a Windows print server (`PrinterName`, `Model`, `Driver`, `PortName`, `IPAddress`). Queues that point at the print server then get their printer's own address in the SNMP form, with the model and driver from the list. The README describes the format and gives an export command.
- A queue finds its printer by the name in its print server device address, or by the queue name macOS makes from it. A direct device address still wins.
- An address comes from `IPAddress`, or from a `PortName` such as `IP_10.0.0.5` or `10.0.0.5_1` when `IPAddress` is `N/A`. UTF-8, UTF-8 with a byte order mark, UTF-16 and the PowerShell 5 `#TYPE` line are accepted.
- Errors: a file that can't be used (empty, not text, invalid CSV with the line number, missing or repeated columns, no usable printers, over 2 MB or 5,000 rows) is refused with the reason, and the old list stays. Rows that can't be used are skipped and listed by line with the reason. The result says how many queues on this Mac found their printer.
- Only the parsed list is saved (`~/.cups-webui/printer-inventory.json`, mode 0600); the CSV is not kept. **Remove list** forgets it.
- New API: `GET`/`POST`/`DELETE /api/v1/accessories/inventory`. The accessories report has `snmp_host_source` and `print_server_entry`.
- The settings files are written by one shared atomic-write helper.

## 2026-11-20

### Server stops when the browser closes

- The server exits about 15 seconds after the last CUPS WebUI tab closes, and no sooner than 2 minutes after it started. It counts open event streams and recent API requests, so a page reload or a second tab does not stop it. A server that no browser connects to exits after 2 minutes. `-keep-running` turns this off.
- The LaunchAgent no longer starts the server at login, and launchd no longer restarts it (`RunAtLoad` false, no `KeepAlive`). Before, launchd restarted the server every time it exited, so it could never stop. Run `launchagent install` again to replace a plist from an earlier version.
- `cups-webui open` starts the installed service when the server is stopped, waits for it, then opens the browser. A bookmark to `http://127.0.0.1:6310` works only while the server runs.
- An instance that exits no longer removes a `session.json` that a later instance wrote. Each start still replaces the file, so `cups-webui open` goes to the instance started last.

### Review fixes for the idle shutdown

- The plist has no `KeepAlive`. The first version used `KeepAlive` with `SuccessfulExit` false, which implies `RunAtLoad` (launchd.plist(5)), so the server still started at every login. A Go program that panics exits with status 2, not by a signal, so `KeepAlive` with `Crashed` would not restart it either. `cups-webui open` is the one way the service starts again. A server that cannot bind its port now exits once; before, launchd restarted it about every 10 seconds.
- `cups-webui open` refuses to open the browser when the process that listens on the port belongs to another user account. With the server stopped the port is free, and another account on the Mac could bind it and serve its own page at the CUPS WebUI address. A bookmark still has no such check; SECURITY.md says so.
- `cups-webui open` no longer exits with an error when the service does not start or no session file appears. It prints a warning and opens the default address, as it did before the idle shutdown. It also skips the start when launchd already runs the service, so a missing `session.json` costs no 5-second wait.
- `cups-webui open` tests the server with an API request, not a bare TCP connection. The request resets the idle timer, so the server cannot stop between the test and the first request of the browser.
- Only an API request that passed the origin and sign-in checks resets the idle timer. Before, a rejected request (cross-site, no token) counted, so any web page or another local user could keep the server running.
- A request no longer ends the 2-minute start window. The test request from `cups-webui open` would have left the browser 15 seconds to open.
- A closing event stream records its time before it leaves the subscriber count. In the old order the server could see no subscriber and an old time, and stop with no grace period.
- The idle timer uses the monotonic clock. A step of the system clock (time sync after sleep, a manual change) no longer delays the shutdown or ends the grace period at once.

### README

- Rewrote the README around the current behaviour. New sections: what the program does, how the server runs (start, stop, login, the free port), commands with the files the program writes, printer drivers, and a table of the 7 on-demand review findings. The speed numbers and the driver list order are now in the README. The stale lines about the start at login are gone.
- Project map: the Composition and History tables are measured again at commit `323254c`. The two diagrams still come from the code graph of 2026-11-19, and the section says so.

### Fixes

- The page no longer logs a Content-Security-Policy error on each load. The inline theme script in `web/index.html` moved to `web/public/theme-init.js`, which `script-src 'self'` allows. The policy is unchanged. A test (`internal/web/embed_test.go`) fails if the built `index.html` has an inline script.

### Drivers

- The Swap Driver and Add Printer lists show first the drivers whose make is in the queue's name or description. Before, the bundled Xerox 5.20.0 PPDs started at row 14, below the five visible rows (DYMO, Epson, Generic, HP), so a queue such as `Office_Xerox` seemed to have no Xerox drivers. "Generic" and "Raw" never count as a match. Other queues keep the `lpinfo` order.

### Speed

Measured on a Mac with 5 queues and a 614 KB `error_log`.

- The log page loads in about 7 ms, down from about 280 ms. The server tested every line of the log against every diagnostic pattern and then kept only the newest lines. It now reads from the newest line back and stops at the number of lines asked for. The entries returned are the same.
- One 5-second refresh of the page reads the printer list once, not twice. The status request and the printer request each ran the same two slow `lpstat` calls. The list is now shared for 1 second, and requests that arrive during a read wait for that read. The status request takes about 255 ms, down from about 400 ms.
- The two slow `lpstat` calls run at the same time. A printer list read with nothing stored takes about 200 ms, down from about 290 ms.
- Every command that changes CUPS (add, modify, delete, pause, resume, default, options, sharing, test page, job actions) drops the shared list, so the page shows the change on its next read. A change made outside CUPS WebUI can show up to 1 second later than before.

## 2026-11-19

- Added `Install CUPS WebUI.command`, a double-click installer. It clears the download flag on `bin/cups-webui`, installs the login service, waits for it to answer, and opens the browser. macOS 27 removed the built-in CUPS web interface, and the unsigned binary does not run from a download until that flag is cleared.
- The installer stops with a message when another copy of CUPS WebUI already holds port 6310. Without that check the service could not start, and the installer would report success against the other copy.
- The installer offers to install a Xerox driver image found in `drivers/` when its version differs from the installed one. Vendor PPDs give the printer's real trays and finishing options, which the generic drivers do not.
- Added `drivers/XeroxDrivers_5.20.0_2566.dmg` (Xerox drivers 5.20.0, SHA-256 `87260d2e...a195bc`), so one download holds the web interface and the drivers.
- Added a "Project map" section to the README: an architecture diagram, a file dependency diagram, and composition and history tables.
- Added `assets/papercut-logo.png` as the installer's Finder icon. The installer re-applies it on each run because git and zip downloads drop Finder icons.
- Added a "One-Click Install" section to the README with the Gatekeeper steps.
- Up to this point, source and `bin/cups-webui` were unchanged from upstream PaperCutSoftware/cups-web-interface (binary blob `033d5d52`). The fixes below change both.

### Fixes

- Saving queue details no longer turns printer sharing off. The server now reads each queue's sharing flag from CUPS, and the page sends the flag only when it was changed.
- A custom PPD uploaded in Add Printer or in the driver swap is now installed. Before, the file was validated and then dropped, so the queue was created without it. A valid upload is kept for one hour in `~/.cups-webui/ppd-uploads` (owner-only) and passed to `lpadmin -P`.
- Job times are read as local time. Before, they were read as UTC, so west of UTC every active job was marked as stuck.
- Held jobs show as held, so the Release button appears. Job states now come from CUPS, not from the job's position in the queue.
- Live updates stay connected. The event stream was cut every 15 seconds by the server's write timeout.
- A failed `lpstat` call is reported as an error. Before, the job list showed as empty.
- PPD validation shows the reasons `cupstestppd` gives. Its real output format was not recognised.
- The test page message shows the job number, and a new message is no longer hidden early by the timer of the previous one.
- Reinstalling the login service stages the new binary before it stops the old service, and no longer deletes the installed binary first. Install and uninstall from the web page run in a separate helper, because the service cannot finish them while stopping itself.
- The LaunchAgent plist escapes paths, so a home folder with `&` in its name gives a valid file.
- `cups-webui open` exits with an error when the browser cannot be opened, so the installer's message for that case can appear.
- Installer: picks the newest bundled driver image by version (5.20.0 after 5.9.0), offers drivers only when they are newer than the installed ones, and reports a disk image that holds no package.

### Security

- The sign-in link, which holds the session token, is printed only to a terminal. It no longer goes to the LaunchAgent log file.
- `-host` accepts only loopback addresses, and `-host ::1` now works. `0.0.0.0` is no longer an accepted `Host` header.
- The `printer` and `which` values of the jobs request are validated before they reach `lpstat`.
- Error messages hide the user name and password of a device address such as `smb://user:password@server/queue`.
- The check for dangerous PPD directives reads lines of any length and both line endings. A line longer than 64 KB used to end the check early.
- A new session token is generated at every start. A token left behind by a crash is not reused.
- A settings request that leaves a setting out keeps that setting's current value.
