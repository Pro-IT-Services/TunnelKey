# Tunnelkey

Open-source OpenVPN client for **Android** and **iOS** built for servers that
use two-factor authentication, plus a small **provisioning server** that puts a
complete, locked-down configuration onto a phone with one scan.

- **Password + TOTP** — for OpenVPN servers whose 2FA plugin expects the
  authenticator code glued to the password (`hunter2` + `123456` →
  `hunter2123456`, or code first). Profiles with `static-challenge` are
  supported too: the code is then sent as the challenge response.
- **Setup codes** — an admin uploads an `.ovpn`, optional username/password,
  the user's TOTP secret and a list of links (websites, Remote Desktop, app
  URIs). The phone scans the QR code(s) and switches to **single-config mode**:
  the configuration name is the title, one Connect button, and the links below.
  With the TOTP secret on the phone, connecting needs no typing at all.
- **Locked secrets** — a stored password or TOTP secret is protected by
  fingerprint / Face ID or an 8-digit PIN that refuses weak patterns.

Native on both platforms: Kotlin + Jetpack Compose on Android, Swift + SwiftUI
+ NetworkExtension on iOS. Both use the OpenVPN 3 core.

```
tunnelkey/
├── android/   Kotlin/Compose app + :ovpn3 module (OpenVPN 3 core via JNI)
├── ios/       SwiftUI app + packet tunnel extension (OpenVPNAdapter)
├── server/    Go provisioning server with embedded admin web UI
└── docs/      Setup code format
```

## How 2FA works

On import (or in the profile editor) turn on **“Server asks for an
authenticator code”** and choose whether the code goes after or before the
password. When connecting, the app asks for the 6- or 8-digit code, combines it
with the password and hands one password to the OpenVPN core. The code is never
stored.

In single-config mode with a provisioned TOTP secret, the phone computes the
code itself (RFC 6238; SHA-1/256/512, 6 or 8 digits, any period) and waits for a
fresh one if the current code is about to expire.

> Servers with `auth-nocache` forget the password after sign-in, so any
> reconnect needs a new sign-in. Push `auth-gen-token` from the server to let
> clients reconnect without a new code.

## Provisioning server

### Docker (recommended)

Needs Docker with the Compose plugin and SSH access to the repository.

```bash
git clone git@github.com:kalipsers/TunnelKey.git /opt/tunnelkey
cd /opt/tunnelkey
cp .env.example .env          # set TUNNELKEY_ADMIN_USER / TUNNELKEY_ADMIN_PASSWORD
./deploy.sh
```

The admin UI listens on port **9897**. To update later just run
`/opt/tunnelkey/deploy.sh` again: it syncs the checkout with `origin/main`,
rebuilds the image, restarts the container, waits for the health check and
removes old images. It refuses to overwrite local changes unless you pass
`--force`; `-b <branch>` deploys another branch, `--no-pull` redeploys the
current checkout, `-h` lists all options.

- Data (SQLite database + `secret.key`) lives in the `tunnelkey_tunnelkey-data`
  volume — back it up as a whole.
- Create or reset an admin at any time:
  `docker exec -i tunnelkey-server tunnelkey-server admin <username>`
  (type the password, then Enter).
- The container runs read-only, as a non-root user, without capabilities.
- Set `TUNNELKEY_BIND=127.0.0.1` in `.env` and put a TLS reverse proxy in front
  (it must send `X-Forwarded-Proto: https`) for anything beyond a trusted LAN.

### Without Docker

```bash
cd server
go build -o tunnelkey-server .
./tunnelkey-server admin -data ./data admin      # create an admin (password on stdin)
./tunnelkey-server -listen 127.0.0.1:8080 -data ./data
```

Or bootstrap the first admin with `TUNNELKEY_ADMIN_USER` /
`TUNNELKEY_ADMIN_PASSWORD` (12+ characters). Put it behind a TLS reverse proxy
(nginx, Caddy) for anything but local use; cookies become `Secure` when the
proxy sends `X-Forwarded-Proto: https`.

In the web UI: **New package** → drop the `.ovpn` → sign-in and 2FA (paste the
base32 secret or the whole `otpauth://` link; a live code preview lets you check
it against the server) → links → **Create & show codes**. Small profiles (EC
keys) fit into one QR code; larger ones (RSA) are split into several, which the
phone scans in any order. *Full screen* cycles through them.

Data lives in `-data`: `tunnelkey.db` (SQLite) and `secret.key`, the AES-256
key that encrypts every package at rest. Back up both, or neither.

**Security model:** setup codes are *not* encrypted — anyone who sees or
photographs one gets that VPN access, including the 2FA secret. Show codes only
to their owner and delete the package once the phone is set up. The format is
documented in [docs/provisioning-format.md](docs/provisioning-format.md).

## App lock

A configuration that includes a password or TOTP secret must be locked:

- **Fingerprint / face** — Android: AES-256-GCM key in the Android Keystore
  usable only after a strong biometric check, invalidated when biometrics
  change. iOS: Keychain item with `.biometryCurrentSet` access control.
- **8-digit PIN** — PBKDF2-SHA256 (310 000 rounds) + AES-GCM, and on Android
  wrapped again with a Keystore key. Refused: fewer than 4 different digits, a
  digit 4+ times, runs like 1234/9876, even steps like 13579, repeated blocks
  (12121212, 12341234), pairs (11223344), mirrors (12344321), dates and common
  keypad shapes. After 5 wrong PINs growing delays apply; the 10th erases the
  configuration.

The app re-locks after 30 seconds in the background. *Remove configuration*
(menu) wipes everything and returns to the normal multi-profile app.

## Building

### Android

Requirements: Android Studio (JDK 17+), SDK 36. The NDK and CMake are fetched
by Gradle; OpenVPN 3, mbed TLS, asio, fmt and LZ4 are downloaded at configure
time and pinned by SHA-256 (see `android/ovpn3/src/main/cpp/CMakeLists.txt`).

```bash
cd android
./gradlew :app:testDebugUnitTest :app:assembleDebug
```

### iOS

Requirements: macOS, Xcode 15+, [XcodeGen](https://github.com/yonaskolb/XcodeGen),
a paid Apple developer account (Network Extensions need the entitlement).

```bash
cd ios
# set DEVELOPMENT_TEAM (and your own bundle IDs + App Group) in project.yml
xcodegen generate
open Tunnelkey.xcodeproj
```

The tunnel uses [OpenVPNAdapter](https://github.com/ss-abramchuk/OpenVPNAdapter)
0.8.0 (OpenVPN 3 + mbed TLS 2) via Swift Package Manager.

## Licence

GNU Affero General Public License v3 — see [LICENSE](LICENSE). The OpenVPN 3
core and OpenVPNAdapter are AGPL-3.0 licensed.
