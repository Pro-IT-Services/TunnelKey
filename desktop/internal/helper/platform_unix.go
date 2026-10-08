//go:build !windows

package helper

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

func openvpnPath() (string, error) {
	candidates := []string{"/usr/sbin/openvpn", "/usr/bin/openvpn", "/usr/local/sbin/openvpn"}
	if runtime.GOOS == "darwin" {
		candidates = []string{
			"/Library/Application Support/Tunnelkey/openvpn/openvpn",
			"/opt/homebrew/sbin/openvpn", "/usr/local/sbin/openvpn",
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", errNoOpenVPN
}

func runBase() string {
	if runtime.GOOS == "darwin" {
		return "/var/run/tunnelkey/sessions"
	}
	return "/run/tunnelkey/sessions"
}

func sessionDir(id int) (string, error) {
	base := runBase()
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(base, 0o700); err != nil {
		return "", err
	}
	dir := filepath.Join(base, "s"+strconv.Itoa(id)+"-"+randomHex(4))
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// management uses a Unix socket inside the private session directory.
func management(dir, pwPath string) ([]string, func() (net.Conn, error), error) {
	sock := filepath.Join(dir, "mgmt.sock")
	if len(sock) > 100 {
		return nil, nil, errors.New("management socket path too long")
	}
	return []string{"--management", sock, "unix", pwPath},
		func() (net.Conn, error) { return net.Dial("unix", sock) }, nil
}

// OpenVPN doesn't configure DNS on Linux or macOS by itself; the helper
// binary doubles as the up/down hook (see dnshook_*.go).
func platformArgs(string) []string {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	hook := self + " dns-hook"
	return []string{"--script-security", "2", "--up", hook, "--down", hook, "--up-restart"}
}

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminate(p *os.Process) { p.Signal(syscall.SIGTERM) }
