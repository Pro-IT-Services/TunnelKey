//go:build !windows

package hello

// Available reports whether Windows Hello can be used.
func Available() bool { return false }

// Verify always fails outside Windows.
func Verify(string, string) error { return ErrUnavailable }

// BiometricsEnrolled is Windows-only.
func BiometricsEnrolled() bool { return false }

// BiometricAvailable is Windows-only.
func BiometricAvailable() bool { return false }
