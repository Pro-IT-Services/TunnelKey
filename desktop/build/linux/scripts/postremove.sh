#!/bin/sh
# Runs after removal and upgrade (deb postrm, rpm %postun).
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
fi

case "$1" in
remove | purge | 0)
	rm -rf /run/tunnelkey
	;;
esac

if command -v update-mime-database >/dev/null 2>&1; then
	update-mime-database /usr/share/mime || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi
exit 0
