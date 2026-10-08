package totp

import (
	"encoding/base32"
	"testing"
	"time"
)

// RFC 6238 appendix B test vectors (8 digits).
func TestRFC6238(t *testing.T) {
	seeds := map[string]string{
		"SHA1":   "12345678901234567890",
		"SHA256": "12345678901234567890123456789012",
		"SHA512": "1234567890123456789012345678901234567890123456789012345678901234",
	}
	cases := []struct {
		at   int64
		want map[string]string
	}{
		{59, map[string]string{"SHA1": "94287082", "SHA256": "46119246", "SHA512": "90693936"}},
		{1111111109, map[string]string{"SHA1": "07081804", "SHA256": "68084774", "SHA512": "25091201"}},
		{20000000000, map[string]string{"SHA1": "65353130", "SHA256": "77737706", "SHA512": "47863826"}},
	}
	for alg, seed := range seeds {
		g, err := New(base32.StdEncoding.EncodeToString([]byte(seed)), 8, 30, alg)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			if got := g.Code(time.Unix(c.at, 0)); got != c.want[alg] {
				t.Errorf("%s at %d: got %s want %s", alg, c.at, got, c.want[alg])
			}
		}
	}
}

func TestSecondsLeft(t *testing.T) {
	g, _ := New("JBSWY3DPEHPK3PXP", 6, 30, "SHA1")
	if s := g.SecondsLeft(time.Unix(61, 0)); s != 29 {
		t.Fatalf("got %d", s)
	}
	if _, err := New("not base32!", 6, 30, ""); err == nil {
		t.Fatal("bad secret accepted")
	}
}
