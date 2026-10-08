// Package ovpn inspects and sanitises OpenVPN client profiles.
//
// The privileged helper runs openvpn as root/SYSTEM, so a profile must not be
// able to run scripts, load plugins or read files on the computer. Sanitize
// enforces that with an allowlist; the app runs the same check on import so
// problems show up before the first connect.
package ovpn

import (
	"fmt"
	"sort"
	"strings"
)

// MaxProfileBytes caps profile size, like the phone app.
const MaxProfileBytes = 256 * 1024

// Summary is what the import form needs to know about a profile.
type Summary struct {
	Remote             string // "host:port/proto" of the first remote, for display
	NeedsCredentials   bool   // contains auth-user-pass
	StaticChallenge    string // text of static-challenge, if any
	HasStaticChallenge bool
}

// Options the helper adds itself, or that only make sense for a server or a
// daemon. They are dropped silently because removing them never weakens the
// connection.
var dropped = map[string]bool{
	"script-security": true, "up": true, "down": true, "route-up": true, "route-pre-down": true,
	"ipchange": true, "up-delay": true, "up-restart": true, "down-pre": true, "tls-export-cert": true,
	"user": true, "group": true, "daemon": true, "log": true, "log-append": true, "status": true,
	"status-version": true, "writepid": true, "syslog": true, "suppress-timestamps": true,
	"machine-readable-output": true, "management": true, "management-hold": true,
	"management-query-passwords": true, "management-client": true, "management-signal": true,
	"management-forget-disconnect": true, "management-up-down": true, "management-log-cache": true,
	"management-client-auth": true, "management-query-proxy": true, "management-query-remote": true,
	"auth-retry": true, "service": true, "mlock": true, "nice": true, "chroot": true, "cd": true,
	"auth-nocache": true, "verb": true, "mute": true, "show-net-up": true, "errors-to-stderr": true,
	"echo": true, "windows-driver": true, "allow-pull-fqdn": true, "dev-node": true,
}

// Client options that are safe to pass through unchanged.
var allowed = map[string]bool{
	"client": true, "tls-client": true, "pull": true, "dev": true, "dev-type": true,
	"proto": true, "proto-force": true, "remote": true, "remote-random": true, "remote-random-hostname": true,
	"port": true, "rport": true, "lport": true, "nobind": true, "bind": true, "local": true, "float": true,
	"resolv-retry": true, "persist-key": true, "persist-tun": true, "persist-remote-ip": true, "persist-local-ip": true,
	"connect-retry": true, "connect-retry-max": true, "connect-timeout": true, "server-poll-timeout": true,
	"explicit-exit-notify": true, "keepalive": true, "ping": true, "ping-restart": true, "ping-exit": true,
	"ping-timer-rem": true, "inactive": true, "session-timeout": true, "hand-window": true, "tran-window": true,
	"tls-timeout": true, "reneg-sec": true, "reneg-bytes": true, "reneg-pkts": true,
	"remote-cert-tls": true, "remote-cert-ku": true, "remote-cert-eku": true, "ns-cert-type": true,
	"verify-x509-name": true, "verify-hash": true, "x509-username-field": true, "x509-track": true,
	"cipher": true, "ciphers": true, "data-ciphers": true, "data-ciphers-fallback": true, "ncp-ciphers": true,
	"ncp-disable": true, "auth": true, "tls-version-min": true, "tls-version-max": true, "tls-cipher": true,
	"tls-ciphersuites": true, "tls-groups": true, "tls-cert-profile": true, "ecdh-curve": true,
	"key-direction": true, "key-method": true, "tls-auth": true, "tls-crypt": true, "tls-crypt-v2": true,
	"ca": true, "cert": true, "key": true, "extra-certs": true, "pkcs12": true, "crl-verify": true, "dh": true,
	"peer-fingerprint": true, "secret": true,
	"comp-lzo": true, "compress": true, "comp-noadapt": true, "allow-compression": true,
	"auth-user-pass": true, "auth-token-user": true, "static-challenge": true, "push-peer-info": true,
	"setenv": true, "setenv-safe": true, "ignore-unknown-option": true, "pull-filter": true,
	"route": true, "route-ipv6": true, "route-gateway": true, "route-ipv6-gateway": true, "route-metric": true,
	"route-delay": true, "route-method": true, "route-nopull": true, "redirect-gateway": true,
	"redirect-private": true, "block-ipv6": true, "block-outside-dns": true, "dhcp-option": true, "dns": true,
	"register-dns": true, "ip-win32": true, "dhcp-renew": true, "dhcp-release": true, "tap-sleep": true,
	"max-routes": true, "allow-recursive-routing": true, "topology": true, "ifconfig": true,
	"ifconfig-ipv6": true, "ifconfig-nowarn": true, "tun-ipv6": true, "tun-mtu": true, "tun-mtu-extra": true,
	"link-mtu": true, "mssfix": true, "fragment": true, "mtu-disc": true, "mtu-test": true, "txqueuelen": true,
	"sndbuf": true, "rcvbuf": true, "fast-io": true, "replay-window": true, "mute-replay-warnings": true,
	"replay-persist": false, "single-session": true, "socks-proxy": true, "socks-proxy-retry": true,
	"http-proxy": true, "http-proxy-option": true, "http-proxy-retry": true, "http-proxy-timeout": true,
	"disable-dco": true, "compat-mode": true, "tls-exit": true, "opt-verify": true, "auth-gen-token": false,
	"route-noexec": true, "ifconfig-noexec": true, "tun-max-mtu": true, "dns-updown": false,
	"disable-occ": true, "occ": true, "passtos": true, "mark": true, "inactive-bytes": false,
	"remap-usr1": true, "tcp-nodelay": true, "auth-token": true,
}

// Options whose argument names a file. Inline blocks are fine; a path would
// make the privileged helper read an arbitrary file.
var fileArg = map[string]bool{
	"ca": true, "cert": true, "key": true, "extra-certs": true, "pkcs12": true, "dh": true,
	"tls-auth": true, "tls-crypt": true, "tls-crypt-v2": true, "crl-verify": true, "secret": true,
}

// Inline blocks that may appear as <tag>…</tag>.
var inlineTags = map[string]bool{
	"ca": true, "cert": true, "key": true, "extra-certs": true, "pkcs12": true, "dh": true,
	"tls-auth": true, "tls-crypt": true, "tls-crypt-v2": true, "crl-verify": true, "secret": true,
	"peer-fingerprint": true, "auth-user-pass": true, "http-proxy-user-pass": true,
}

// Error explains why a profile was refused.
type Error struct {
	Line   int
	Option string
	Reason string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Reason)
	}
	return e.Reason
}

// Inspect reads what the import form shows. It does not validate.
func Inspect(content string) Summary {
	var s Summary
	walk(content, func(_ int, parts []string) {
		switch strings.ToLower(parts[0]) {
		case "remote":
			if s.Remote == "" && len(parts) >= 2 {
				port := "1194"
				if len(parts) >= 3 {
					port = parts[2]
				}
				s.Remote = parts[1] + ":" + port
				if len(parts) >= 4 {
					s.Remote += "/" + parts[3]
				}
			}
		case "auth-user-pass":
			s.NeedsCredentials = true
		case "static-challenge":
			s.HasStaticChallenge = true
			if len(parts) >= 2 {
				s.StaticChallenge = parts[1]
			}
		}
	}, func(tag string) {
		if tag == "auth-user-pass" {
			s.NeedsCredentials = true
		}
	})
	return s
}

// Sanitize checks a profile against the allowlist and returns the version the
// helper hands to openvpn: options it manages itself are removed and the file
// argument of auth-user-pass is dropped (credentials come from the app).
func Sanitize(content string) (string, error) {
	content = Normalize(content)
	if len(content) > MaxProfileBytes {
		return "", &Error{Reason: "the profile is larger than 256 KB"}
	}
	var out strings.Builder
	var inBlock string // inline tag currently open
	var connectionDepth int
	sawRemote := false

	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		n := i + 1
		line := strings.TrimSpace(raw)

		if inBlock != "" {
			out.WriteString(raw)
			out.WriteByte('\n')
			if strings.EqualFold(line, "</"+inBlock+">") {
				inBlock = ""
			}
			continue
		}
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			tag := strings.ToLower(strings.Trim(line, "</>"))
			closing := strings.HasPrefix(line, "</")
			switch {
			case tag == "connection":
				if closing {
					connectionDepth--
				} else {
					connectionDepth++
				}
				if connectionDepth < 0 || connectionDepth > 1 {
					return "", &Error{Line: n, Reason: "unbalanced <connection> block"}
				}
			case closing:
				return "", &Error{Line: n, Reason: fmt.Sprintf("unexpected %s", line)}
			case inlineTags[tag]:
				inBlock = tag
			default:
				return "", &Error{Line: n, Option: tag, Reason: fmt.Sprintf("inline block <%s> is not supported", tag)}
			}
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}

		parts := Tokenize(line)
		if len(parts) == 0 {
			continue
		}
		opt := strings.ToLower(strings.TrimPrefix(parts[0], "--"))
		switch {
		case dropped[opt]:
			continue
		case !allowed[opt]:
			return "", &Error{Line: n, Option: opt, Reason: fmt.Sprintf("option %q is not allowed in Tunnelkey profiles", opt)}
		}
		if fileArg[opt] && len(parts) >= 2 && parts[1] != "[inline]" && !(opt == "dh" && parts[1] == "none") {
			return "", &Error{Line: n, Option: opt, Reason: fmt.Sprintf("%q refers to a file; put it inline as <%s>…</%s>", opt, opt, opt)}
		}
		switch opt {
		case "auth-user-pass":
			// Credentials always come from the app over the management interface.
			if len(parts) > 1 {
				line = "auth-user-pass"
			}
		case "http-proxy":
			// host port [authfile|'auto'|'auto-nct'] [auth-method]
			if len(parts) >= 4 && parts[3] != "auto" && parts[3] != "auto-nct" && parts[3] != "[inline]" {
				return "", &Error{Line: n, Option: opt, Reason: "http-proxy credentials must be inline (<http-proxy-user-pass>)"}
			}
		case "socks-proxy":
			if len(parts) >= 4 && parts[3] != "[inline]" {
				return "", &Error{Line: n, Option: opt, Reason: "socks-proxy credentials must be inline"}
			}
		case "remote":
			sawRemote = true
		case "dev":
			if len(parts) >= 2 && !strings.HasPrefix(parts[1], "tun") && !strings.HasPrefix(parts[1], "tap") {
				return "", &Error{Line: n, Option: opt, Reason: "only tun and tap devices are supported"}
			}
		case "setenv", "setenv-safe":
			if len(parts) >= 2 && strings.EqualFold(parts[1], "opt") {
				// "setenv opt X" marks X as optional; X itself still has to be allowed.
				if len(parts) >= 3 {
					inner := strings.ToLower(parts[2])
					if !allowed[inner] && !dropped[inner] {
						continue // openvpn would ignore it anyway
					}
				}
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if inBlock != "" {
		return "", &Error{Reason: fmt.Sprintf("<%s> is not closed", inBlock)}
	}
	if !sawRemote {
		return "", &Error{Reason: "the profile has no remote server"}
	}
	return out.String(), nil
}

// Normalize unifies newlines and removes a byte-order mark.
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimPrefix(s, "\uFEFF")
}

// Tokenize splits a directive line, honouring double/single quotes.
func Tokenize(line string) []string {
	var out []string
	var sb strings.Builder
	var quote rune
	escaped, has := false, false
	for _, c := range line {
		switch {
		case escaped:
			sb.WriteRune(c)
			escaped = false
		case c == '\\' && quote == '"':
			escaped = true
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
			has = true
		case quote == 0 && (c == ' ' || c == '\t'):
			if sb.Len() > 0 || has {
				out = append(out, sb.String())
				sb.Reset()
				has = false
			}
		case quote == 0 && (c == '#' || c == ';') && sb.Len() == 0 && !has:
			// Trailing comment.
			return out
		default:
			sb.WriteRune(c)
		}
	}
	if sb.Len() > 0 || has {
		out = append(out, sb.String())
	}
	return out
}

func walk(content string, directive func(line int, parts []string), block func(tag string)) {
	var inBlock string
	for i, raw := range strings.Split(Normalize(content), "\n") {
		line := strings.TrimSpace(raw)
		if inBlock != "" {
			if strings.EqualFold(line, "</"+inBlock+">") {
				inBlock = ""
			}
			continue
		}
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			tag := strings.ToLower(strings.Trim(line, "</>"))
			if tag != "connection" && !strings.HasPrefix(line, "</") {
				inBlock = tag
				block(tag)
			}
			continue
		}
		if parts := Tokenize(line); len(parts) > 0 {
			directive(i+1, parts)
		}
	}
}

// AllowedOptions lists the options passed through, for documentation.
func AllowedOptions() []string {
	var out []string
	for k, v := range allowed {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
