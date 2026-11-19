#!/bin/bash
# Installs the bundled cups-webui as a login service, then opens it in the browser.
set -u

cd "$(dirname "$0")" || exit 1
BIN="bin/cups-webui"
URL="http://127.0.0.1:6310"

fail() {
  printf '\nERROR: %s\nPress Return to close.\n' "$1"
  read -r _
  exit 1
}

[ -x "$BIN" ] || fail "bin/cups-webui is missing. Keep this file inside the folder it came in."

# A Finder icon lives in the resource fork, which git and zip downloads drop, so put it back.
osascript -l JavaScript -e 'ObjC.import("AppKit"); function run(a) { $.NSWorkspace.sharedWorkspace.setIconForFileOptions($.NSImage.alloc.initWithContentsOfFile(a[0]), a[1], 0) }' \
  "$PWD/assets/papercut-logo.png" "$PWD/$(basename "$0")" >/dev/null 2>&1

# The binary is not notarized. Gatekeeper refuses to run it while the download flag is set.
xattr -d com.apple.quarantine "$BIN" 2>/dev/null

# A copy started by hand holds the port, so the service could not start and the check below would pass against the wrong copy.
if curl -s -o /dev/null --max-time 1 "$URL" && ! launchctl list com.papercut.cups-webui >/dev/null 2>&1; then
  fail "Another copy of CUPS WebUI already uses $URL. Close the Terminal window that runs it, then run this installer again."
fi

echo "Installing CUPS WebUI..."
"$BIN" launchagent install || fail "The install command failed. See the message above."

up=0
for _ in $(seq 1 20); do
  if curl -s -o /dev/null --max-time 1 "$URL"; then up=1; break; fi
  sleep 0.5
done
[ "$up" = 1 ] || fail "The service did not answer at $URL. Check ~/Library/Logs for com.papercut.cups-webui logs."

# 'open' prints a sign-in link that holds a session token, so keep it off the screen.
"$BIN" open >/dev/null || fail "Could not open the browser. Go to $URL yourself."

echo "CUPS WebUI is running at $URL and starts at every login."
echo "To remove it: ./bin/cups-webui launchagent uninstall"

# Optional: a Xerox driver disk image in drivers/, named XeroxDrivers_<version>_<build>.dmg.
DMG=$(find drivers -maxdepth 1 -name 'XeroxDrivers_*.dmg' 2>/dev/null | sort -V | tail -1)
[ -n "$DMG" ] || exit 0
bundled=$(basename "$DMG" | cut -d_ -f2)
installed=$(lpinfo -m 2>/dev/null | sed -n 's/.* Xerox .*, \([0-9.]*\)$/\1/p' | head -1)
echo
echo "Xerox print drivers: installed ${installed:-none}, bundled $bundled."
# Offer the bundled drivers only when they are newer than the installed ones.
[ "$installed" = "$bundled" ] && exit 0
[ -n "$installed" ] && [ "$(printf '%s\n%s\n' "$installed" "$bundled" | sort -V | tail -1)" = "$installed" ] && exit 0
printf 'Install Xerox drivers %s? This asks for your password. [y/N] ' "$bundled"
read -r answer
[ "$answer" = y ] || [ "$answer" = Y ] || exit 0
volume=$(hdiutil attach -nobrowse -readonly "$DMG" | awk -F'\t' 'END { print $NF }')
[ -d "$volume" ] || fail "Could not open $DMG."
package=$(find "$volume" -maxdepth 1 -name '*.pkg' | head -1)
if [ -z "$package" ]; then
  hdiutil detach -quiet "$volume"
  fail "No installer package found in $DMG."
fi
sudo installer -pkg "$package" -target /
result=$?
hdiutil detach -quiet "$volume"
[ "$result" = 0 ] || fail "The Xerox driver install failed."
echo "Xerox drivers $bundled installed."
