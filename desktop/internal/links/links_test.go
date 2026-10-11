package links

import "testing"

func TestParseRDP(t *testing.T) {
	r, err := ParseRDP("rdp://full%20address=s:10.0.0.5:3389&username=s:marko")
	if err != nil || r.Address != "10.0.0.5:3389" || r.Username != "marko" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := ParseRDP("rdp://full%20address=s:a%0Aalternate%20shell:s:cmd"); err == nil {
		t.Fatal("newline accepted")
	}
}

// Open must accept what the setup pages produce, including RDP links
// (which Go's URL parser rejects).
func TestOpenDispatches(t *testing.T) {
	var gotURL string
	var gotRDP RDP
	launchURL = func(u string) error { gotURL = u; return nil }
	launchRDP = func(r RDP, _, _ string) error { gotRDP = r; return nil }
	defer func() { launchURL, launchRDP = openURL, openRDP }()

	if err := Open("rdp", "rdp://full%20address=s:172.26.100.35:3389&username=s:marko", "WRK"); err != nil {
		t.Fatalf("rdp: %v", err)
	}
	if gotRDP.Address != "172.26.100.35:3389" || gotRDP.Username != "marko" {
		t.Fatalf("rdp target %+v", gotRDP)
	}
	if err := Open("rdp", "rdp://full%20address=s:pc01.corp.local:3390&username=s:CORP%5Cmarko", "PC"); err != nil || gotRDP.Username != `CORP\marko` {
		t.Fatalf("rdp with domain user: %v %+v", err, gotRDP)
	}
	if err := Open("rdp", "rdp://full%20address=s:10.0.0.5:3389", "No user"); err != nil || gotRDP.Username != "" {
		t.Fatalf("rdp without user: %v %+v", err, gotRDP)
	}
	if err := Open("web", " https://intranet.example.com/a?b=1 ", "Intranet"); err != nil || gotURL != "https://intranet.example.com/a?b=1" {
		t.Fatalf("web: %v %q", err, gotURL)
	}
	if err := Open("app", "myapp://open?server=x", "App"); err != nil || gotURL != "myapp://open?server=x" {
		t.Fatalf("app: %v %q", err, gotURL)
	}
}

func TestRefusesUnsafe(t *testing.T) {
	for _, c := range [][2]string{
		{"web", "file:///C:/Windows/System32/cmd.exe"}, {"web", "javascript:alert(1)"},
		{"app", "file:///etc/passwd"}, {"app", "ms-msdt:/id x"}, {"app", `C:\Windows\notepad.exe`},
		{"app", "search-ms:query=x"}, {"rdp", "https://example.com"},
	} {
		if err := Open(c[0], c[1], "x"); err != ErrUnsafe {
			t.Errorf("%v: %v", c, err)
		}
	}
}
