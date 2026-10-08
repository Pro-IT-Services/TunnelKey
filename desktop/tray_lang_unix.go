//go:build !windows && !darwin

package main

func systemLanguages() []string { return envLanguages() }
