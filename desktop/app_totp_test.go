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

// Without Windows Hello fingerprint/face the 2FA secret is never stored.
func TestTOTPNotStoredWithoutBiometrics(t *testing.T) {
	a := testApp(t, false)
	a.pendingSetup = provisioned()
	if err := a.InstallSetup("hello", ""); err == nil || err.Error() != "hello_unavailable" {
		t.Fatalf("hello without biometrics: %v", err)
	}
	if err := a.InstallSetup("pin", testPin); err != nil {
		t.Fatal(err)
	}
	m := a.store.Managed()
	if m.HasTOTP || !m.ManualCode {
		t.Fatalf("managed %+v", m)
	}
	r, err := a.vault.UnlockWithPin(testPin)
	if err != nil || r.Secrets.TOTPSecret != "" || r.Secrets.Password != "pw" {
		t.Fatalf("vault %+v %v", r.Secrets, err)
	}
}

// A secret stored behind a PIN by an earlier version is erased at unlock.
func TestPinUnlockDropsOldTOTPSecret(t *testing.T) {
	a := testApp(t, false)
	p := store.Profile{ID: "m1", Name: "Office", Managed: true, TwoFactor: true, CodeLength: 6, NeedsCredentials: true}
	a.store.SaveProfile(p, "client\nremote x\n")
	a.store.SetManaged(&store.Managed{ProfileID: "m1", Name: "Office", HasTOTP: true, HasPassword: true, TOTPDigits: 6, TOTPPeriod: 30})
	a.vault.Store(vault.Pin, testPin, vault.Secrets{Password: "pw", TOTPSecret: "JBSWY3DPEHPK3PXP"})

	r, err := a.UnlockWithPin(testPin)
	if err != nil || !r.OK {
		t.Fatalf("%+v %v", r, err)
	}
	if a.secrets.TOTPSecret != "" || a.store.Managed().HasTOTP || !a.store.Managed().ManualCode {
		t.Fatal("2FA secret kept")
	}
	again, _ := a.vault.UnlockWithPin(testPin)
	if again.Secrets == nil || again.Secrets.TOTPSecret != "" || again.Secrets.Password != "pw" {
		t.Fatalf("vault after drop: %+v", again.Secrets)
	}
}

// With biometrics but a PIN chosen, codes are typed too.
func TestPinChoiceDropsTOTPEvenWithBiometrics(t *testing.T) {
	a := testApp(t, true)
	a.pendingSetup = provisioned()
	if err := a.InstallSetup("pin", testPin); err != nil {
		t.Fatal(err)
	}
	if m := a.store.Managed(); m.HasTOTP || !m.ManualCode {
		t.Fatalf("managed %+v", m)
	}
}
