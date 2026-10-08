package main

import "testing"

func TestCredentialComposition(t *testing.T) {
	cases := []struct {
		act                activeSession
		password, response string
	}{
		{activeSession{password: "hunter2"}, "hunter2", ""},
		{activeSession{password: "hunter2", twoFactor: true, codeAfter: true}, "hunter2123456", ""},
		{activeSession{password: "hunter2", twoFactor: true}, "123456hunter2", ""},
		{activeSession{password: "hunter2", twoFactor: true, codeAfter: true, static: true}, "hunter2", "123456"},
	}
	for i, c := range cases {
		c.act.username = "marko"
		got := c.act.credentials("123456")
		if c.act.twoFactor == false {
			got = c.act.credentials("")
		}
		if got.Username != "marko" || got.Password != c.password || got.Response != c.response {
			t.Errorf("case %d: %+v", i, got)
		}
	}
}

func TestValidCode(t *testing.T) {
	if !validCode("123456", 6) || validCode("12345", 6) || validCode("12345a", 6) || !validCode("12345678", 8) {
		t.Fatal("validCode")
	}
}

func TestLooksLikePlainPackage(t *testing.T) {
	if !looksLikePlainPackage([]byte(`{"name":"Office","ovpn":"client\nremote x\n","password":"pw"}`)) ||
		looksLikePlainPackage([]byte(`{"tunnelkey":"setup-file","v":1}`)) || looksLikePlainPackage([]byte("client\nremote x\n")) {
		t.Fatal("looksLikePlainPackage")
	}
}

func TestLooksLikeSetupFile(t *testing.T) {
	if !looksLikeSetupFile([]byte("\xef\xbb\xbf{\n  \"tunnelkey\": \"setup-file\",")) || looksLikeSetupFile([]byte("client\nremote x\n")) {
		t.Fatal("looksLikeSetupFile")
	}
}
