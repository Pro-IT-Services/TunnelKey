//go:build !windows

package main

func systemLanguages() []string { return envLanguages() }
