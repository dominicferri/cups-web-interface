# Changelog

## 2026-11-19

- Added `Install CUPS WebUI.command`, a double-click installer. It clears the download flag on `bin/cups-webui`, installs the login service, waits for it to answer, and opens the browser. macOS 27 removed the built-in CUPS web interface, and the unsigned binary does not run from a download until that flag is cleared.
- Added `assets/papercut-logo.png` as the installer's Finder icon. The installer re-applies it on each run because git and zip downloads drop Finder icons.
- Added a "One-Click Install" section to the README with the Gatekeeper steps.
- Source and `bin/cups-webui` are unchanged from upstream PaperCutSoftware/cups-web-interface (binary blob `033d5d52`).
