# Tunnelkey Privacy Policy

**Effective date:** 26 September 2026 · **Last updated:** 8 October 2026
**Applies to:** the Tunnelkey apps for Android and iOS, the Tunnelkey desktop app for Windows, macOS and Linux, the Tunnelkey setup page, and the self-hosted Tunnelkey provisioning server software
**Publisher:** ProIT services — contact: [develop@pro-it.sk](mailto:develop@pro-it.sk)

## Summary

Tunnelkey is a VPN *client*. It connects your device to an OpenVPN server that
you or your organisation operate. **ProIT services does not run VPN servers and
does not receive, collect, store or sell any data from the apps.** There are no
accounts, no analytics, no advertising and no tracking.

## 1. Data the apps process on your device

To work, the apps store the following **only on your device**:

| Data | Purpose | Protection |
|------|---------|------------|
| VPN profiles (`.ovpn` files, including certificates and private keys) | Connecting to your VPN server | Phones: app-private storage. Desktop: encrypted with a key protected by your operating-system account (see below) |
| Username | Signing in to your VPN server | App-private storage |
| Password (only if you choose to save it, or your administrator provisions it) | Signing in to your VPN server | Encrypted with a key in the Android Keystore / iOS Keychain, or on desktop with your account's key store |
| Two-factor (TOTP) secret (only when provisioned by your administrator) | Generating sign-in codes on the device | Encrypted; phones: protected by fingerprint/face or an 8-digit PIN. Desktop: stored only on Windows with Windows Hello fingerprint or face recognition — otherwise it is not stored at all and you type the code each time |
| Links added by your administrator | Opening intranet sites, Remote Desktop or other apps | App-private storage |
| Connection log (connection events, server address, errors) | Showing you what happened, for troubleshooting | Kept in memory; shared only if *you* copy or share it |

On desktop computers the encryption key is protected by the operating system:
Windows Data Protection (DPAPI) on Windows, the login Keychain on macOS, and the
Secret Service (for example GNOME Keyring or KWallet) on Linux. If a Linux
system has no Secret Service, the key is protected only by file permissions and
the app tells you so. The desktop app keeps its data in your user profile
(`%APPDATA%\Tunnelkey` on Windows, `~/Library/Application Support/Tunnelkey` on
macOS, `~/.config/Tunnelkey` on Linux).

Authenticator codes you type are used for a single sign-in and are never stored.

The apps read the clipboard only when you choose *Paste code* or *Paste from
clipboard*, to take a code or a profile you copied yourself; nothing is read in
the background.

This data is **not transmitted to ProIT services or any third party**. On
phones it is excluded from cloud backups and device transfers. You can delete
it at any time by deleting a profile, using *Remove configuration*, or
uninstalling the app (on desktop, also delete the folder listed above).
Entering a wrong PIN ten times erases the provisioned configuration.

## 2. Network traffic and the VPN

When you connect, the app creates an encrypted OpenVPN tunnel **between your
device and the VPN server in your profile**: through Android's `VpnService`,
iOS's Network Extension, or on desktop through the Tunnelkey helper service
running OpenVPN 2.6 (community edition). Your credentials and one-time code are
sent only to that server, over the encrypted connection, to sign you in. On
desktop they are passed from the app to the helper over a local channel on the
same computer and are not written to disk.

Tunnelkey does not inspect, log, modify, redirect or sell your traffic. What
happens to traffic after it reaches your VPN server is governed by the operator
of that server (you or your organisation), not by ProIT services.

## 3. Permissions

**Phones**

| Permission | Why |
|------------|-----|
| VPN (`BIND_VPN_SERVICE`, user consent dialog) | Creating the VPN tunnel — the app's core function |
| Internet | Connecting to your VPN server |
| Camera | Scanning setup QR codes. Images are analysed on the device and never stored or sent anywhere |
| Biometrics (fingerprint / face) | Unlocking stored secrets. Biometric data is handled entirely by the operating system; the app never receives it |
| Notifications and foreground service | Showing the ongoing VPN connection and a Disconnect button |

**Desktop**

| Component | Why |
|-----------|-----|
| Tunnelkey helper (system service) | Creating the VPN tunnel, network routes and DNS settings, which require administrator rights. It only runs profiles that pass a safety check (no scripts, plugins or file access) and keeps a short technical log of its own start-up and errors on the computer |
| OpenVPN 2.6 and its network drivers (installed with Tunnelkey on Windows; the system's `openvpn` package on Linux) | The OpenVPN engine and virtual network adapter |
| Windows Hello | Unlocking stored secrets. Face, fingerprint and PIN checks are handled entirely by Windows; the app only receives "verified" or "not verified" |

## 4. Setup codes, setup files and the provisioning server

Organisations can use the open-source Tunnelkey provisioning server to create
setup QR codes (for phones) and password-encrypted setup files (`.tunnelkey`,
for desktop computers). That server is **installed and operated by the
organisation itself**, not by ProIT services. The organisation decides what a
setup code or file contains (VPN profile, username, password, TOTP secret,
links) and is the controller of that data; please contact your IT administrator
about how they handle it. Setup codes and files are read on the device; opening
them does not contact any server.

Setup files are encrypted in the web browser with a password chosen by the
administrator (PBKDF2-SHA256 and AES-256-GCM). The password is not sent to the
provisioning server.

Setup codes and files can also be created with the Tunnelkey setup page
(<https://pro-it-services.github.io/TunnelKey/provision/>). It runs entirely in
the web browser: what you enter is processed on your computer, never
transmitted (the page is technically prevented from making network requests)
and not stored unless you save a file yourself. GitHub, which hosts the page,
may log ordinary web-server data such as your IP address when the page is
loaded; see GitHub's privacy statement.

## 5. Third parties

The apps contain no third-party analytics, advertising or tracking SDKs. Links
provisioned by your administrator may open other apps (for example a web
browser, Remote Desktop Connection or Microsoft's Windows App); those apps have
their own privacy policies. The desktop app and its installer do not check for
updates or contact ProIT services.

## 6. Children

Tunnelkey is a tool for connecting to organisational or personal VPN servers and
is not directed at children under 13 (or the equivalent minimum age in your
country).

## 7. Security

Secrets are encrypted with hardware-backed or operating-system-protected keys,
the apps can be locked with biometrics, Windows Hello or a strong PIN, and the
source code is public for independent review:
<https://github.com/Pro-IT-Services/TunnelKey>.

## 8. Your rights

Because ProIT services does not collect or hold personal data from the apps,
there is no data for us to access, correct or delete on our side. All data can
be removed from the device as described in section 1. For data held by your VPN
or provisioning server, contact its operator. Under the GDPR you may also lodge
a complaint with your supervisory authority (in Slovakia: Úrad na ochranu
osobných údajov SR).

## 9. Changes

If this policy changes, the new version will be published at this address with
a new "last updated" date. Material changes will also be noted in the apps'
release notes.

## 10. Contact

ProIT services — [develop@pro-it.sk](mailto:develop@pro-it.sk)
