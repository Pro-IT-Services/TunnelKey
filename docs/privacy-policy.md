# Tunnelkey Privacy Policy

**Effective date:** 26 September 2026
**Applies to:** the Tunnelkey apps for Android and iOS, and the self-hosted Tunnelkey provisioning server software
**Publisher:** ProIT services — contact: [develop@pro-it.sk](mailto:develop@pro-it.sk)

## Summary

Tunnelkey is a VPN *client*. It connects your device to an OpenVPN server that
you or your organisation operate. **ProIT services does not run VPN servers and
does not receive, collect, store or sell any data from the app.** There are no
accounts, no analytics, no advertising and no tracking.

## 1. Data the app processes on your device

To work, the app stores the following **only on your device**:

| Data | Purpose | Protection |
|------|---------|------------|
| VPN profiles (`.ovpn` files, including certificates and private keys) | Connecting to your VPN server | App-private storage |
| Username | Signing in to your VPN server | App-private storage |
| Password (only if you choose to save it, or your administrator provisions it) | Signing in to your VPN server | Encrypted with a key in the Android Keystore / iOS Keychain |
| Two-factor (TOTP) secret (only when provisioned by your administrator) | Generating sign-in codes on the device | Encrypted; protected by fingerprint/face or an 8-digit PIN |
| Links added by your administrator | Opening intranet sites, Remote Desktop or other apps | App-private storage |
| Connection log (connection events, server address, errors) | Showing you what happened, for troubleshooting | Kept in memory / app storage; shared only if *you* copy or share it |

Authenticator codes you type are used for a single sign-in and are never stored.

This data is **not transmitted to ProIT services or any third party**. It is
excluded from cloud backups and device transfers. You can delete it at any time
by deleting a profile, using *Remove configuration*, or uninstalling the app.
Entering a wrong PIN ten times erases the provisioned configuration.

## 2. Network traffic and the VPN

When you connect, the app uses Android's `VpnService` (or iOS's Network
Extension) to create an encrypted OpenVPN tunnel **between your device and the
VPN server in your profile**. Your credentials and one-time code are sent only
to that server, over the encrypted connection, to sign you in.

Tunnelkey does not inspect, log, modify, redirect or sell your traffic. What
happens to traffic after it reaches your VPN server is governed by the operator
of that server (you or your organisation), not by ProIT services.

## 3. Permissions

| Permission | Why |
|------------|-----|
| VPN (`BIND_VPN_SERVICE`, user consent dialog) | Creating the VPN tunnel — the app's core function |
| Internet | Connecting to your VPN server |
| Camera | Scanning setup QR codes. Images are analysed on the device and never stored or sent anywhere |
| Biometrics (fingerprint / face) | Unlocking stored secrets. Biometric data is handled entirely by the operating system; the app never receives it |
| Notifications and foreground service | Showing the ongoing VPN connection and a Disconnect button |

## 4. Setup codes and the provisioning server

Organisations can use the open-source Tunnelkey provisioning server to create
setup QR codes. That server is **installed and operated by the organisation
itself**, not by ProIT services. The organisation decides what a setup code
contains (VPN profile, username, password, TOTP secret, links) and is the
controller of that data; please contact your IT administrator about how they
handle it. Setup codes are read on the device; scanning does not contact any
server.

## 5. Third parties

The app contains no third-party analytics, advertising or tracking SDKs. Links
provisioned by your administrator may open other apps (for example a web
browser or Microsoft's Windows App for Remote Desktop); those apps have their
own privacy policies.

## 6. Children

Tunnelkey is a tool for connecting to organisational or personal VPN servers and
is not directed at children under 13 (or the equivalent minimum age in your
country).

## 7. Security

Secrets are encrypted with hardware-backed keys where the device supports them,
the app can be locked with biometrics or a strong PIN, and the source code is
public for independent review: <https://github.com/kalipsers/TunnelKey>.

## 8. Your rights

Because ProIT services does not collect or hold personal data from the app,
there is no data for us to access, correct or delete on our side. All data can
be removed from the device as described in section 1. For data held by your VPN
or provisioning server, contact its operator. Under the GDPR you may also lodge
a complaint with your supervisory authority (in Slovakia: Úrad na ochranu
osobných údajov SR).

## 9. Changes

If this policy changes, the new version will be published at this address with
a new effective date. Material changes will also be noted in the app's release
notes.

## 10. Contact

ProIT services — [develop@pro-it.sk](mailto:develop@pro-it.sk)
