package protect

import "testing"

func TestSealRoundTrip(t *testing.T) {
	s, err := newSealer(newKey())
	if err != nil {
		t.Fatal(err)
	}
	sealed := s.Seal([]byte("secret"), []byte("aad"))
	if got, err := s.Open(sealed, []byte("aad")); err != nil || string(got) != "secret" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := s.Open(sealed, []byte("other")); err == nil {
		t.Fatal("wrong aad accepted")
	}
}
