# Changelog

## 2026-11-19

- Added `Install CUPS WebUI.command`, a double-click installer. It clears the download flag on `bin/cups-webui`, installs the login service, waits for it to answer, and opens the browser. macOS 27 removed the built-in CUPS web interface, and the unsigned binary does not run from a download until that flag is cleared.
- The installer stops with a message when another copy of CUPS WebUI already holds port 6310. Without that check the service could not start, and the installer would report success against the other copy.
- The installer offers to install a Xerox driver image found in `drivers/` when its version differs from the installed one. Vendor PPDs give the printer's real trays and finishing options, which the generic drivers do not.
- Added `drivers/XeroxDrivers_5.20.0_2566.dmg` (Xerox drivers 5.20.0, SHA-256 `87260d2e...a195bc`), so one download holds the web interface and the drivers.
- Added `assets/papercut-logo.png` as the installer's Finder icon. The installer re-applies it on each run because git and zip downloads drop Finder icons.
- Added a "One-Click Install" section to the README with the Gatekeeper steps.
- Source and `bin/cups-webui` are unchanged from upstream PaperCutSoftware/cups-web-interface (binary blob `033d5d52`).
