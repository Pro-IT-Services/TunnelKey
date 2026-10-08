# build/

Packaging inputs for the Tunnelkey desktop app. See `../README.md` for the
build commands.

- `appicon.png` - 1024 px app icon (Wails: macOS icns, Linux). Regenerate all
  icons with `../scripts/make-icons.py`.
- `windows/` - `icon.ico`, `info.json` and `wails.exe.manifest` (used by
  `wails build`), `installer/Tunnelkey.wxs` (MSI), `installer/Bundle.wxs`
  (setup.exe with OpenVPN), `installer/logo.png`.
- `darwin/` - `Info.plist` / `Info.dev.plist` (Wails templates with the
  document types), the launchd daemon plist, `pkg/` (installer scripts and
  productbuild distribution) and `uninstall.sh`.
- `linux/` - `nfpm.yaml`, systemd unit, `.desktop` entry, shared-mime-info
  XML, hicolor icons and package maintainer scripts.

Generated (not committed): `bin/` (wails/go output), `out/` (packages),
`cache/` (downloaded OpenVPN MSI, macOS source tarballs and build trees).
