package setupfile

import (
	"bytes"
	"os"
	"testing"
)

func sample() *Payload {
	return &Payload{
		Version: 1, Name: "Office VPN", OVPN: "client\nremote vpn.example.com 1194\n",
		Username: "marko", TOTP: &TOTP{Secret: "JBSWY3DPEHPK3PXP", Digits: 6, Period: 30, Algorithm: "SHA1"},
		CodePosition: "a", Links: []Link{{Title: "Intranet", Kind: "web", URI: "https://intranet.example.com"}},
	}
}

func TestRoundTrip(t *testing.T) {
	file, err := Encrypt(sample(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decrypt(file, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Office VPN" || p.TOTP.Secret != "JBSWY3DPEHPK3PXP" || len(p.Links) != 1 || !p.CodeAfter() {
		t.Fatalf("unexpected payload %+v", p)
	}
}

func TestRejects(t *testing.T) {
	file, _ := Encrypt(sample(), "correct horse battery")
	if _, err := Decrypt(file, "wrong password!"); err != ErrWrongPassword {
		t.Fatalf("wrong password: %v", err)
	}
	tampered := bytes.Replace(file, []byte(`"iter": 600000`), []byte(`"iter": 100000`), 1)
	if _, err := Decrypt(tampered, "correct horse battery"); err != ErrWrongPassword {
		t.Fatalf("tampered iter: %v", err)
	}
	if _, err := Decrypt([]byte("client\nremote x\n"), "x"); err != ErrNotSetupFile {
		t.Fatalf("ovpn: %v", err)
	}
	newer := bytes.Replace(file, []byte(`"v": 1`), []byte(`"v": 2`), 1)
	if _, err := Decrypt(newer, "correct horse battery"); err != ErrNewerVersion {
		t.Fatalf("newer: %v", err)
	}
}

// The setup pages encrypt in the browser; this file was produced by their
// JavaScript (docs/provision/core.js) and is shared with the server tests.
func TestDecryptsBrowserFile(t *testing.T) {
	file, err := os.ReadFile("../../../server/testdata/setup_v1.tunnelkey")
	if err != nil {
		t.Skip("shared test vector not found:", err)
	}
	p, err := Decrypt(file, "correct-horse-battery-staple-07")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name == "" || p.OVPN == "" {
		t.Fatalf("unexpected payload %+v", p)
	}
	t.Logf("decrypted %q with %d links, totp=%v", p.Name, len(p.Links), p.TOTP != nil)
}
