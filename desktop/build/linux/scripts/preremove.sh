#!/bin/sh
# Runs before removal (deb prerm: remove|upgrade|deconfigure|failed-upgrade;
# rpm %preun: $1 = 0 on erase, 1 on upgrade). Only stop on real removal.
set -e

case "$1" in
remove | purge | 0)
	if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
		systemctl disable --now tunnelkey-helper.service || true
	fi
	;;
esac
exit 0
