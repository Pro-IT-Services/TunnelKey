#!/bin/sh
# Runs after install and upgrade (deb: configure; rpm: %post with $1 = 1 or 2).
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
	systemctl enable tunnelkey-helper.service || true
	# restart = start on first install, pick up the new binary on upgrade.
	systemctl restart tunnelkey-helper.service || true
fi

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
