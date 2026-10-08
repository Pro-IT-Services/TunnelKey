//go:build !windows

package hello

// Available reports whether Windows Hello can be used.
func Available() bool { return false }

// Verify always fails outside Windows.
func Verify(string, string) error { return ErrUnavailable }
