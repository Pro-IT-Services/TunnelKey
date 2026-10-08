# Tunnelkey for Windows, macOS and Linux

Tunnelkey is an OpenVPN client with two-factor sign-in, published by ProIT
services under the AGPL-3.0 (see `../LICENSE`). The desktop app imports
encrypted `.tunnelkey` setup files from the Tunnelkey provisioning server and
plain OpenVPN profiles (`.ovpn`), keeps the secrets on the computer and
connects through OpenVPN 2.6. It is a client only: it does not provide a VPN
service.

Contact: develop@pro-it.sk

## Architecture

```
Tunnelkey (GUI, user)  --IPC-->  tunnelkey-helper (service, root/SYSTEM)  --spawns-->  openvpn 2.6
   Wails v2 + web UI              named pipe / Unix socket                     management interface
```

- **GUI** (`main.go`, `app.go`, `frontend/`): a Wails v2 app running as the
  logged-in user. It stores profiles and secrets, asks for the TOTP code or
  password, and talks to the helper. It never needs administrator rights.
- **Privileged helper** (`cmd/tunnelkey-helper`, `internal/helper`): a small
  system service that starts and stops `openvpn`, answers its management
  interface (credentials, challenge/response) and applies pushed DNS settings
  (`dns-hook`: systemd-resolved on Linux, `scutil` on macOS). Endpoints:
  - Windows: service `TunnelkeyHelper` (LocalSystem), named pipe
    `\\.\pipe\tunnelkey-helper`; management on loopback TCP with a password file.
  - macOS: launchd daemon `app.tunnelkey.helper`, socket `/var/run/tunnelkey/helper.sock`.
  - Linux: systemd unit `tunnelkey-helper.service`, socket `/run/tunnelkey/helper.sock`.
  Per-connection files (management password, socket) live in a directory only
  root/SYSTEM can read.
- **Profile allowlist** (`internal/ovpn`): because openvpn runs privileged, every
  profile is sanitised before it is started. Options that run scripts, load
  plugins or read local files are rejected or dropped; the app runs the same
  check at import so problems show up early.
- **OpenVPN 2.6**:
  - Windows: the official OpenVPN community MSI, installed by `Tunnelkey-<ver>-setup.exe`
    when missing (only `openvpn.exe`, its interactive service and the
    ovpn-dco / Wintun / TAP-Windows6 drivers; no OpenVPN GUI). Found through
    `HKLM\SOFTWARE\OpenVPN\exe_path`.
  - macOS: built from source by `scripts/build-macos.sh`, statically linked with
    OpenSSL 3, LZ4 and LZO, installed at
    `/Library/Application Support/Tunnelkey/openvpn/openvpn`.
  - Linux: the distribution package (`openvpn >= 2.5`, 2.6 recommended),
    `/usr/sbin/openvpn` or `/usr/bin/openvpn`.

## Tray icon

Every platform shows a tray icon (menu bar item on macOS): grey when not
connected, brass while connecting, green when protected. The menu has the
status, Open, Connect/Disconnect and Quit; on Windows and Linux a left click
opens the window. Closing the window only hides it; **Quit** in the tray menu
disconnects and ends the app. Launching Tunnelkey again also brings the window
back.

- Windows, Linux: `fyne.io/systray` (`tray_systray.go`). Linux needs a
  StatusNotifier host (KDE and most desktops; GNOME with the AppIndicator
  extension).
- macOS: a small AppKit status item (`tray_darwin.m`) instead of a tray
  library, because those replace the NSApplication delegate Wails depends on.
- Shared logic: `tray.go`. Icons: `scripts/make-tray-icons.py` → `trayicons/`.

## File types

| Type | Windows | macOS | Linux |
|------|---------|-------|-------|
| `.tunnelkey` (`application/vnd.tunnelkey.setup+json`) | default handler | owner (UTI `com.proitservices.tunnelkey.setup`) | default (`tunnelkey.desktop` + shared-mime-info) |
| `.ovpn` | "Open with" only | alternate viewer | listed in `MimeType=` (`application/x-openvpn-profile`) |

The file path is passed to the GUI as the first command-line argument
(Windows, Linux) or through the macOS open-document event.

## Building

All scripts read the version from `wails.json` (`info.productVersion`, `x.y.z`)
and write packages to `build/out/`. Downloads go to `build/cache/` and are
verified against SHA-256 sums pinned in the scripts. `build/bin`, `build/out`
and `build/cache` are not committed.

Common requirements: Go (version in `go.mod`), Node.js + npm, Wails CLI:

```
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

Development: `wails dev` (the helper must be running separately, as root or as
the Windows service, for connections to work).

Tests: `go vet ./...` and `go test ./...` (on Linux add `-tags webkit2_41`).
`frontend/dist/gitkeep` stays committed so `//go:embed all:frontend/dist`
compiles before the frontend has been built.

### Windows (MSI + setup.exe)

Requirements: WiX Toolset 4.0.6 and its extensions:

```
dotnet tool install --global wix --version 4.0.6
wix extension add -g WixToolset.UI.wixext/4.0.6
wix extension add -g WixToolset.Util.wixext/4.0.6
wix extension add -g WixToolset.Bal.wixext/4.0.6
```

```
powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
```

Outputs:

- `Tunnelkey-<ver>-x64.msi`: per-machine MSI (Program Files\Tunnelkey, the
  `TunnelkeyHelper` service, Start menu shortcut, file associations). It does
  not contain OpenVPN.
- `Tunnelkey-<ver>-setup.exe`: the installer to ship. Installs the official
  OpenVPN MSI (pinned version, SHA-256 and Authenticode checked at build time)
  when OpenVPN is missing or older, then the Tunnelkey MSI.

Code signing is off by default. Set `SIGN_CERT_SHA1` (certificate thumbprint)
or `SIGN_PFX` + `SIGN_PFX_PASSWORD`, and optionally `SIGNTOOL` and
`SIGN_TIMESTAMP_URL`; the script then signs both executables, the MSI, the Burn
engine and the bundle. `-PlaceholderGui` builds the installers with a stub
`Tunnelkey.exe` (installer testing only), `-SkipGui` reuses `build\bin\Tunnelkey.exe`.

### Linux (.deb + .rpm)

Ubuntu 24.04 / Debian 13 build host:

```
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
bash scripts/build-linux.sh
```

The GUI is built with `-tags webkit2_41` (WebKitGTK 4.1). Packages are made
with nfpm (`build/linux/nfpm.yaml`; the script runs a pinned nfpm through
`go run` if it is not installed). Runtime dependencies: `openvpn (>= 2.5)`,
WebKitGTK 4.1 and GTK 3; `systemd-resolved` is recommended for DNS.

### macOS (.pkg)

Requirements: Xcode command line tools (no Homebrew packages are used).

```
bash scripts/build-macos.sh
```

Builds a universal app and helper, compiles OpenVPN for arm64 and x86_64 from
the official tarball, checks with `otool -L` that only system libraries are
linked, and produces `Tunnelkey-<ver>.pkg` (macOS 12+). Signing and
notarization run only when `APPLE_APP_IDENTITY`, `APPLE_INSTALLER_IDENTITY` and
`APPLE_NOTARY_PROFILE` (or `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_PASSWORD`)
are set.

### CI

`.github/workflows/desktop.yml` runs on `workflow_dispatch` and on `desktop-v*`
tags: vet + tests and an unsigned build on Windows, Ubuntu 24.04 and macOS 14,
with the packages uploaded as workflow artifacts.

### Icons

`scripts/make-icons.py` (Pillow) regenerates `build/appicon.png`,
`build/windows/icon.ico`, the Linux hicolor icons and the setup logo from the
shared 1024 px app icon in `ios/`.

## Installing and uninstalling

- **Windows**: run `Tunnelkey-<ver>-setup.exe` (administrator rights required).
  Uninstall "Tunnelkey" from Settings > Apps; this removes the app and the
  helper service. OpenVPN is left installed because other software may use
  it; remove "OpenVPN" separately if it is no longer needed.
- **macOS**: open `Tunnelkey-<ver>.pkg`. The app goes to `/Applications`, the
  helper to `/Library/PrivilegedHelperTools/app.tunnelkey.helper`
  (launchd: `/Library/LaunchDaemons/app.tunnelkey.helper.plist`, log in
  `/Library/Logs/Tunnelkey/`). Uninstall with
  `sudo sh "/Library/Application Support/Tunnelkey/uninstall.sh"`.
- **Linux**: `sudo apt install ./tunnelkey_<ver>-1_amd64.deb` or
  `sudo dnf install ./tunnelkey-<ver>-1.x86_64.rpm`. The package enables and
  starts `tunnelkey-helper.service`. Remove with `apt remove tunnelkey` /
  `dnf remove tunnelkey`.

Uninstalling never deletes the per-user data below.

## Deploying with Group Policy

Group Policy software installation only deploys MSI packages and transforms, so
`scripts/build-windows.ps1` also writes `build\out\gpo\`:

| File | Purpose |
|------|---------|
| `OpenVPN-2.6.x-amd64.msi` | The official, signed OpenVPN MSI (unchanged) |
| `openvpn-gpo.mst` | Installs only the OpenVPN core and its drivers (ovpn-dco on Windows 10 2004+, Wintun, TAP), no OpenVPN GUI |
| `Tunnelkey-<ver>-x64.msi` | Tunnelkey |
| `tunnelkey-gpo.mst` | Sets `SKIPOPENVPNCHECK=1`, so the packages may install in either order |
| `README-GPO.txt` | Step-by-step instructions |

1. Copy the folder to a share the computers can read (UNC path).
2. In a GPO linked to the computers' OU: *Computer Configuration → Policies →
   Software Settings → Software installation → New → Package*.
3. Add the OpenVPN MSI with **Advanced** deployment and `openvpn-gpo.mst` on the
   *Modifications* tab; then the Tunnelkey MSI with `tunnelkey-gpo.mst`.
   Transforms can only be added when the package is created.
4. Computers install both at the next restart. For updates, add the new
   Tunnelkey MSI and mark it as upgrading the previous package (*Upgrades* tab).

On its own (without the transform) the Tunnelkey MSI refuses to install when
OpenVPN is missing and points to `Tunnelkey-<ver>-setup.exe`. For Intune, SCCM
or scripts, `Tunnelkey-<ver>-setup.exe /quiet /norestart` installs both.

## Where data is stored

Per user (profiles, sealed secrets, settings):

- Windows: `%APPDATA%\Tunnelkey`
- macOS: `~/Library/Application Support/Tunnelkey`
- Linux: `~/.config/Tunnelkey` (`$XDG_CONFIG_HOME/Tunnelkey`)

The master key that seals this data is kept with DPAPI (Windows), in the
login keychain (macOS) or in the Secret Service keyring (Linux; a 0600 file
when no keyring is available). The helper keeps only short-lived session files
(Windows `%ProgramData%\Tunnelkey\run`, macOS `/var/run/tunnelkey`, Linux
`/run/tunnelkey`).

## Security notes

- Only the helper runs privileged. It runs nothing but the installed OpenVPN
  binary, with profiles that passed the allowlist; any local user may talk
  to it, so the allowlist is the security boundary.
- Management interfaces are bound to a private socket (macOS/Linux) or to
  loopback with a random password (Windows).
- The bundled OpenVPN (Windows MSI, macOS sources) is pinned by version and
  SHA-256 in the build scripts; update the pins for each OpenVPN security
  release.
- The systemd unit applies hardening (`NoNewPrivileges`, `ProtectSystem=full`,
  `ProtectHome=read-only`, a limited capability set) that still allows tun
  devices, routes, ovpn-dco and `resolvectl`.
- Report security issues to develop@pro-it.sk.
