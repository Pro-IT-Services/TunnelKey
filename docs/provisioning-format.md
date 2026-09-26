# Setup code format (v1)

A *setup code* carries a complete, ready-to-use configuration from the
Tunnelkey server to a phone: the OpenVPN profile, optional credentials, an
optional TOTP secret and a list of links. Scanning it switches the app to
**single-config mode**.

> **Security:** setup codes are not encrypted. Anyone who can see or photograph
> a code gets everything in it — including the private key, password and TOTP
> secret. Show codes only to the person they are for, and delete the package on
> the server once the phone is set up.

## 1. Payload

UTF-8 JSON with short keys:

```json
{
  "v": 1,
  "n": "Office VPN",
  "o": "client\ndev tun\n…<ca>…</ca>…",
  "u": "marko",
  "p": "s3cret",
  "t": { "s": "JBSWY3DPEHPK3PXP", "d": 6, "p": 30, "a": "SHA1" },
  "f": false,
  "c": "a",
  "l": [
    { "t": "Intranet", "k": "web", "u": "https://intranet.example.com" },
    { "t": "My PC",    "k": "rdp", "u": "rdp://full%20address=s:10.0.0.5:3389&username=s:marko" }
  ]
}
```

| Key | Required | Meaning |
|-----|----------|---------|
| `v` | yes | Format version, `1`. |
| `n` | yes | Configuration name, shown as the app title. |
| `o` | yes | Complete `.ovpn` profile, certificates and keys inline. |
| `u` | no  | Username for `auth-user-pass`. |
| `p` | no  | Password. If absent the user is asked for it on connect. |
| `t` | no  | TOTP settings (RFC 6238). `s` base32 secret, `d` digits (6 or 8), `p` period in seconds, `a` `SHA1`/`SHA256`/`SHA512`. When present the app generates the code itself. |
| `f` | no  | `true` = the server needs a code but no secret is provisioned; the user types the code. Ignored when `t` is present. |
| `c` | no  | Code placement: `a` = after the password (default), `b` = before. |
| `l` | no  | Links. `t` title, `k` kind (`web`, `rdp`, `app`), `u` URI opened by the OS. |

RDP links use the Microsoft Remote Desktop URI scheme
(`rdp://full%20address=s:host:port&username=s:user`), handled by Microsoft's
Remote Desktop / Windows App clients on iOS and Android.

## 2. Encoding

1. Serialise the payload as JSON (UTF-8).
2. Compress with **zlib** (RFC 1950; deflate + Adler-32 checksum).
3. Encode the bytes with **Base45** (RFC 9285). Base45 uses only the QR
   *alphanumeric* character set, so codes are dense and are returned as plain
   text by every scanner.
4. Split the Base45 text into `n` parts and wrap each one:

```
TK1:<i>/<n>:<ID>:<len>:<data>
```

- `i` — part number, 1-based; `n` — total parts.
- `ID` — 6 characters from `0-9A-Z`, identical in all parts of one code set.
- `len` — number of characters in `data`. Base45 may end with a space, which
  some scanners trim; readers pad `data` with spaces back to `len`.
- `data` — the slice of Base45 text.

Readers collect parts in any order, ignore parts with a different `ID`, join
`data` in part order, Base45-decode, zlib-inflate (verifying Adler-32), and
parse the JSON. Unknown keys must be ignored.

The server puts everything in one QR code (error correction L) when the Base45
text is at most 2 900 characters, otherwise it splits it into parts of at most
1 500 characters (error correction M).
