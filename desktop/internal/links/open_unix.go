//go:build !windows

package links

import (
	"os/exec"
	"runtime"
)

func openURL(u string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("/usr/bin/open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}

// openRDP hands the rdp:// URI to Microsoft's Windows App on macOS; on Linux
// it prefers FreeRDP when installed.
func openRDP(r RDP, uri, _ string) error {
	if runtime.GOOS != "darwin" {
		for _, bin := range []string{"xfreerdp3", "xfreerdp", "wlfreerdp"} {
			if path, err := exec.LookPath(bin); err == nil {
				args := []string{"/v:" + r.Address}
				if r.Username != "" {
					args = append(args, "/u:"+r.Username)
				}
				return exec.Command(path, args...).Start()
			}
		}
	}
	return openURL(uri)
}
