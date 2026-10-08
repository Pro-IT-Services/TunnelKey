package vault

import (
	"testing"
	"time"

	"github.com/kalipsers/TunnelKey/desktop/internal/protect"
)

func newVault(t *testing.T) *Vault {
	s, _, err := protect.OpenSealer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return Open(t.TempDir(), s)
}

func TestPinVault(t *testing.T) {
	v := newVault(t)
	now := time.Unix(1_700_000_000, 0)
	v.now = func() time.Time { return now }
	if err := v.Store(Pin, "48159263", Secrets{TOTPSecret: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatal(err)
	}
	if v.Method() != Pin {
		t.Fatal("method")
	}
	for i := 1; i <= 4; i++ {
		r, err := v.UnlockWithPin("00000000")
		if err != ErrWrongPin || r.AttemptsLeft != MaxAttempts-i || r.LockedUntil.After(now) {
			t.Fatalf("attempt %d: %+v %v", i, r, err)
		}
	}
	r, _ := v.UnlockWithPin("00000000") // 5th: 30 s delay
	if !r.LockedUntil.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("delay: %+v", r)
	}
	if _, err := v.UnlockWithPin("48159263"); err != ErrWrongPin {
		t.Fatal("unlocked during lock-out")
	}
	now = now.Add(31 * time.Second)
	r, err := v.UnlockWithPin("48159263")
	if err != nil || r.Secrets.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("%+v %v", r, err)
	}
	if f, _ := v.Status(); f != 0 {
		t.Fatal("failures not reset")
	}
}

func TestPinVaultWipes(t *testing.T) {
	v := newVault(t)
	now := time.Unix(1_700_000_000, 0)
	v.now = func() time.Time { return now }
	v.Store(Pin, "48159263", Secrets{Password: "x"})
	var r PinResult
	for i := 0; i < MaxAttempts; i++ {
		now = now.Add(2 * time.Hour)
		r, _ = v.UnlockWithPin("11111111")
	}
	if !r.Wiped || v.Method() != "" {
		t.Fatalf("not wiped: %+v", r)
	}
}

func TestHelloVault(t *testing.T) {
	v := newVault(t)
	v.Store(Hello, "", Secrets{Password: "pw"})
	s, err := v.Open()
	if err != nil || s.Password != "pw" {
		t.Fatalf("%+v %v", s, err)
	}
}
