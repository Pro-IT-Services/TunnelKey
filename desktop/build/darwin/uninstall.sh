#!/bin/sh
# Removes Tunnelkey from macOS. Run with: sudo sh uninstall.sh
# Installed as "/Library/Application Support/Tunnelkey/uninstall.sh".
# Per-user data in ~/Library/Application Support/Tunnelkey is kept.
set -u

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root: sudo sh $0" >&2
	exit 1
fi

LABEL=app.tunnelkey.helper
/bin/launchctl bootout "system/$LABEL" 2>/dev/null || true
/bin/rm -f "/Library/LaunchDaemons/$LABEL.plist"
/bin/rm -f "/Library/PrivilegedHelperTools/$LABEL"
/bin/rm -rf "/Applications/Tunnelkey.app"
/bin/rm -rf "/Library/Logs/Tunnelkey"
/bin/rm -rf /var/run/tunnelkey
/usr/sbin/pkgutil --forget com.proitservices.tunnelkey.pkg >/dev/null 2>&1 || true
/bin/rm -rf "/Library/Application Support/Tunnelkey"
echo "Tunnelkey removed."
