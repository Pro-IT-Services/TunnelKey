package store

import "testing"

func TestProfilesAndSecrets(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := Profile{ID: NewID(), Name: "Office", Remote: "vpn:1194/udp", TwoFactor: true, CodeAfter: true, CodeLength: 6}
	if err := s.SaveProfile(p, "client\nremote vpn\n"); err != nil {
		t.Fatal(err)
	}
	if cfg, err := s.Config(p.ID); err != nil || cfg != "client\nremote vpn\n" {
		t.Fatalf("%q %v", cfg, err)
	}
	s.SetPassword(p.ID, "hunter2")
	if s.Password(p.ID) != "hunter2" {
		t.Fatal("password")
	}
	if err := s.DeleteProfile(p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Profiles(); len(list) != 0 || s.Password(p.ID) != "" {
		t.Fatal("not deleted")
	}
}
