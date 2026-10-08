package ovpn

import (
	"strings"
	"testing"
)

const sample = "\uFEFFclient\r\ndev tun\r\nproto udp\r\nremote vpn.example.com 1194 udp\r\n" +
	"auth-user-pass creds.txt\r\nscript-security 2\r\nup /etc/openvpn/update-resolv-conf\r\n" +
	"static-challenge \"Enter code\" 1\r\n<ca>\r\n-----BEGIN CERTIFICATE-----\r\nAAA\r\n-----END CERTIFICATE-----\r\n</ca>\r\n" +
	"key-direction 1\r\n<tls-auth>\r\nkey\r\n</tls-auth>\r\nverb 3\r\n"

func TestInspect(t *testing.T) {
	s := Inspect(sample)
	if s.Remote != "vpn.example.com:1194/udp" || !s.NeedsCredentials || !s.HasStaticChallenge || s.StaticChallenge != "Enter code" {
		t.Fatalf("unexpected summary %+v", s)
	}
}

func TestSanitizeDropsScriptsAndAuthFile(t *testing.T) {
	out, err := Sanitize(sample)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"script-security", "update-resolv-conf", "creds.txt", "verb 3", "\r"} {
		if strings.Contains(out, bad) {
			t.Errorf("sanitised profile still contains %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{"auth-user-pass\n", "<ca>\n", "-----BEGIN CERTIFICATE-----", "</tls-auth>", "remote vpn.example.com 1194 udp"} {
		if !strings.Contains(out, want) {
			t.Errorf("sanitised profile lacks %q:\n%s", want, out)
		}
	}
}

func TestSanitizeRefuses(t *testing.T) {
	base := "client\nremote a.example 1194\n"
	for name, extra := range map[string]string{
		"plugin":         "plugin /tmp/evil.so\n",
		"file ca":        "ca /etc/shadow\n",
		"file tls-auth":  "tls-auth ta.key 1\n",
		"config include": "config other.conf\n",
		"verify script":  "tls-verify /bin/sh\n",
		"unknown":        "frobnicate yes\n",
		"proxy authfile": "http-proxy p.example 8080 /root/pw\n",
		"bad block":      "<script>\nx\n</script>\n",
		"open block":     "<ca>\nAAA\n",
		"providers":      "providers legacy default\n",
		"tmp-dir":        "tmp-dir /\n",
	} {
		if _, err := Sanitize(base + extra); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Sanitize("client\ndev tun\n"); err == nil {
		t.Error("profile without remote accepted")
	}
}

func TestSanitizeConnectionBlocksAndInline(t *testing.T) {
	in := "client\n<connection>\nremote a.example 1194 udp\n</connection>\n<connection>\nremote b.example 443 tcp\n</connection>\n" +
		"tls-crypt [inline]\n<tls-crypt>\nk\n</tls-crypt>\ndh none\nauth-user-pass\nsetenv opt block-outside-dns\n"
	out, err := Sanitize(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "<connection>") != 2 || !strings.Contains(out, "tls-crypt [inline]") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize(`static-challenge "Enter \"the\" code" 1 # comment`)
	if len(got) != 3 || got[1] != `Enter "the" code` || got[2] != "1" {
		t.Fatalf("got %q", got)
	}
}
