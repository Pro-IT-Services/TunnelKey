package main

import (
	"os/exec"
	"strings"
)

// systemLanguages reads the macOS preferred languages (apps started from
// Finder have no LANG variable).
func systemLanguages() []string {
	if l := envLanguages(); len(l) > 0 {
		return l
	}
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	var langs []string
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.Trim(strings.TrimSpace(line), `",()`)
		if l != "" {
			langs = append(langs, l)
		}
	}
	return langs
}
