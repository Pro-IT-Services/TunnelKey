package main

import (
	"testing"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/setupfile"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/store"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/vault"
)

const testPin = "48159263"

func testApp(t *testing.T, biometrics bool) *App {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &App{store: st, vault: vault.Open(st.Dir(), st.Sealer()), drafts: map[string]*draft{}, helloAvailable: biometrics}
}

func provisioned() *setupfile.Payload {
	return &setupfile.Payload{
		Version: 1, Name: "Office", OVPN: "client\nremote vpn.example.com 1194\nauth-user-pass\n",
		Username: "marko", Password: "pw",
		TOTP: &setupfile.TOTP{Secret: "JBSWY3DPEHPK3PXP", Digits: 6, Period: 30, Algorithm: "SHA1"},
	}
}

// Without Windows Hello fingerprint/face: no lock, no 2FA secret, password kept.
func TestNoBiometricsMeansNoLockAndNoSecret(t *testing.T) {
	a := testApp(t, false)
	a.pendingSetup = provisioned()
	sum := a.summaryFor(a.pendingSetup)
	if sum.CanUseHello || !sum.TOTPNeedsHello {
		t.Fatalf("flags %+v", sum)
	}
	if err := a.InstallSetup("hello"); err == nil || err.Error() != "hello_unavailable" {
		t.Fatalf("hello without biometrics: %v", err)
	}
	if err := a.InstallSetup("pin"); err == nil {
		t.Fatal("PIN accepted")
	}
	if err := a.InstallSetup("none"); err != nil {
		t.Fatal(err)
	}
	if a.vault.Method() != vault.None || a.State().Locked {
		t.Fatalf("locked: method %q", a.vault.Method())
	}
	if m := a.store.Managed(); m.HasTOTP || !m.ManualCode || !m.HasPassword {
		t.Fatalf("managed %+v", m)
	}
	sec, err := a.vault.Open()
	if err != nil || sec.TOTPSecret != "" || sec.Password != "pw" {
		t.Fatalf("vault %+v %v", sec, err)
	}
}

// A PIN lock from an earlier version is removed at its last unlock, with the 2FA secret.
func TestLegacyPinUnlockRemovesLock(t *testing.T) {
	a := testApp(t, false)
	p := store.Profile{ID: "m1", Name: "Office", Managed: true, TwoFactor: true, CodeLength: 6, NeedsCredentials: true}
	a.store.SaveProfile(p, "client\nremote x\n")
	a.store.SetManaged(&store.Managed{ProfileID: "m1", Name: "Office", HasTOTP: true, HasPassword: true, TOTPDigits: 6, TOTPPeriod: 30})
	a.vault.Store(vault.Pin, testPin, vault.Secrets{Password: "pw", TOTPSecret: "JBSWY3DPEHPK3PXP"})
	if !a.State().Locked {
		t.Fatal("legacy PIN vault should start locked")
	}
	r, err := a.UnlockWithPin(testPin)
	if err != nil || !r.OK {
		t.Fatalf("%+v %v", r, err)
	}
	if a.vault.Method() != vault.None || a.store.Managed().HasTOTP || !a.store.Managed().ManualCode {
		t.Fatalf("method %q managed %+v", a.vault.Method(), a.store.Managed())
	}
	a.Lock()
	if a.State().Locked {
		t.Fatal("still locks after the last PIN unlock")
	}
	sec, _ := a.vault.Open()
	if sec.TOTPSecret != "" || sec.Password != "pw" {
		t.Fatalf("vault %+v", sec)
	}
}

// Without a 2FA secret in the file, Windows Hello isn't involved at all.
func TestNoTOTPMeansNoHello(t *testing.T) {
	a := testApp(t, true)
	p := provisioned()
	p.TOTP = nil
	a.pendingSetup = p
	if f := a.summaryFor(p); f.CanUseHello {
		t.Fatal("Hello offered without a 2FA secret")
	}
	if err := a.InstallSetup("hello"); err != nil { // falls back to no lock
		t.Fatal(err)
	}
	if a.vault.Method() != vault.None {
		t.Fatalf("method %q", a.vault.Method())
	}
}
