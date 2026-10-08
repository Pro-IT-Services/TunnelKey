//go:build !windows

package main

// launchd and systemd run the helper in the foreground.
func runAsService() bool { return false }
