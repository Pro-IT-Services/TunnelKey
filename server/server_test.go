package main

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestBase45RFCVectors(t *testing.T) {
	cases := map[string]string{
		"AB":      "BB8",
		"Hello!!": "%69 VD92EX0",
		"base-45": "UJCLQE7W581",
		"ietf!":   "QED8WEX0",
	}
	for in, want := range cases {
		if got := base45Encode([]byte(in)); got != want {
			t.Errorf("encode(%q) = %q, want %q", in, got, want)
		}
		back, err := base45Decode(want)
		if err != nil || string(back) != in {
			t.Errorf("decode(%q) = %q, %v", want, back, err)
		}
	}
	if _, err := base45Decode("GGW"); err == nil {
		t.Error("expected overflow error")
	}
}

// RFC 6238 appendix B, 8 digits.
func TestTOTPRFCVectors(t *testing.T) {
	seed20 := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	seed32 := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZA"
	seed64 := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNA"
	cases := []struct {
		secret, algo string
		at           int64
		want         string
	}{
		{seed20, "SHA1", 59, "94287082"},
		{seed32, "SHA256", 59, "46119246"},
		{seed64, "SHA512", 59, "90693936"},
		{seed20, "SHA1", 1111111109, "07081804"},
		{seed20, "SHA1", 20000000000, "65353130"},
	}
	for _, c := range cases {
		cfg := TOTPConfig{Secret: c.secret, Digits: 8, Period: 30, Algorithm: c.algo}
		if err := cfg.normalize(); err != nil {
			t.Fatal(err)
		}
		got, _ := cfg.code(time.Unix(c.at, 0))
		if got != c.want {
			t.Errorf("%s@%d = %s, want %s", c.algo, c.at, got, c.want)
		}
	}
}

func TestOTPAuthURI(t *testing.T) {
	cfg, err := parseOTPAuthURI("otpauth://totp/VPN:marko?secret=jbsw y3dp ehpk3pxp&issuer=VPN&digits=8&period=60&algorithm=sha256")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Secret != "JBSWY3DPEHPK3PXP" || cfg.Digits != 8 || cfg.Period != 60 || cfg.Algorithm != "SHA256" {
		t.Errorf("unexpected %+v", cfg)
	}
	if _, err := parseOTPAuthURI("otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP"); err == nil {
		t.Error("HOTP should be rejected")
	}
}

func testPackage(ovpnSize int) *Package {
	// Incompressible filler, like base64 key material.
	noise := make([]byte, ovpnSize*3/4)
	rand.Read(noise)
	enc := base64.StdEncoding.EncodeToString(noise)
	var body strings.Builder
	for i := 0; i < len(enc); i += 64 {
		body.WriteString(enc[i:min(i+64, len(enc))] + "\n")
	}
	return &Package{
		Name:     "Office",
		OVPN:     "client\r\nremote vpn.example.com 1194 udp\r\n<ca>\r\n" + body.String() + "</ca>\r\n",
		Username: "marko",
		Password: "pässwörd",
		TOTP:     &TOTPConfig{Secret: "JBSWY3DPEHPK3PXP", Digits: 6, Period: 30, Algorithm: "SHA1"},
		Links: []Link{
			{Title: "Intranet", Kind: "web", URL: "https://intranet.example.com"},
			{Title: "My PC", Kind: "rdp", Host: "10.0.0.5", Username: `CORP\marko`},
		},
	}
}

func TestSetupCodeRoundTrip(t *testing.T) {
	for _, size := range []int{500, 6000} {
		p := testPackage(size)
		codes, err := encodeSetupCodes(buildPayload(p))
		if err != nil {
			t.Fatal(err)
		}
		if size == 500 && len(codes) != 1 {
			t.Errorf("small profile: %d codes, want 1", len(codes))
		}
		if size == 6000 && len(codes) < 2 {
			t.Errorf("large profile: %d codes, want several", len(codes))
		}
		for _, c := range codes {
			if len(c) > singleCodeMax+30 {
				t.Errorf("code too long: %d", len(c))
			}
		}
		// Reverse order + simulate a scanner trimming trailing spaces.
		scanned := make([]string, 0, len(codes))
		for i := len(codes) - 1; i >= 0; i-- {
			scanned = append(scanned, strings.TrimRight(codes[i], " "))
		}
		got, err := decodeSetupCodes(scanned)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Office" || got.Password != "pässwörd" || got.TOTP.Secret != "JBSWY3DPEHPK3PXP" ||
			strings.Contains(got.OVPN, "\r") || len(got.Links) != 2 || got.CodePosition != "a" {
			t.Errorf("round trip mismatch: %+v", got)
		}
		if got.Links[1].URI != "rdp://full%20address=s:10.0.0.5:3389&username=s:CORP%5Cmarko" {
			t.Errorf("rdp uri = %s", got.Links[1].URI)
		}
	}
}

func TestApplyRequestValidation(t *testing.T) {
	good := packageRequest{Name: "X", OVPN: "client\nremote a 1194\n"}
	if err := applyRequest(&Package{}, &good, nil); err != nil {
		t.Fatal(err)
	}
	bad := []packageRequest{
		{Name: "", OVPN: good.OVPN},
		{Name: "X", OVPN: "client\n"},
		{Name: "X", OVPN: "client\nremote a\nca ca.crt\n"},
		{Name: "X", OVPN: good.OVPN, Links: []Link{{Title: "a", Kind: "web", URL: "javascript:alert(1)"}}},
		{Name: "X", OVPN: good.OVPN, Links: []Link{{Title: "a", Kind: "app", URL: "javascript:alert(1)"}}},
		{Name: "X", OVPN: good.OVPN, Links: []Link{{Title: "a", Kind: "rdp", Host: "a b"}}},
		{Name: "X", OVPN: good.OVPN, TOTP: &TOTPConfig{Secret: "not base32!"}},
	}
	for i, req := range bad {
		if err := applyRequest(&Package{}, &req, nil); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(h, "correct horse battery") || checkPassword(h, "wrong") {
		t.Error("password check broken")
	}
}
