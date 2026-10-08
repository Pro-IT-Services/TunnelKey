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
