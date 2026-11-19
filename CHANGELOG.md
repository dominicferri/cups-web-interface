# Changelog

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
