//go:build !windows

package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// SocketPath of the helper.
func SocketPath() string {
	if runtime.GOOS == "darwin" {
		return "/var/run/tunnelkey/helper.sock"
	}
	return "/run/tunnelkey/helper.sock"
}

// Dial connects to the helper.
func Dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return d.DialContext(c, "unix", SocketPath())
}

// Listen opens the helper's endpoint. Any local user may connect; the helper
// only runs profiles that pass the allowlist in package ovpn.
func Listen() (net.Listener, error) {
	path := SocketPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
