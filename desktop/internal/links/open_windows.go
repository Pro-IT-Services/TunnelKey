package links

import (
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/sys/windows"
)

func openURL(u string) error {
	return shellOpen(u)
}

func shellOpen(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, t, nil, nil, windows.SW_SHOWNORMAL)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9 ._-]+`)

// openRDP writes a small .rdp file and opens it with Remote Desktop Connection.
func openRDP(r RDP, _ string, title string) error {
	dir := filepath.Join(os.TempDir(), "Tunnelkey")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := unsafeName.ReplaceAllString(title, "_")
	if name == "" {
		name = "Remote Desktop"
	}
	path := filepath.Join(dir, name+".rdp")
	content := "full address:s:" + r.Address + "\r\nprompt for credentials:i:1\r\n"
	if r.Username != "" {
		content += "username:s:" + r.Username + "\r\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return shellOpen(path)
}
