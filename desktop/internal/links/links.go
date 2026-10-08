// Package links opens the web, Remote Desktop and app links of a provisioned
// configuration.
package links

import (
	"errors"
	"net/url"
	"strings"
)

// ErrUnsafe refuses links that could start programs or open local files.
var ErrUnsafe = errors.New("this link can't be opened safely")

// Schemes that run code or reach local files when handed to the OS.
var blocked = map[string]bool{
	"file": true, "javascript": true, "vbscript": true, "data": true, "shell": true,
	"ms-msdt": true, "search-ms": true, "search": true, "ms-officecmd": true, "ms-excel": false,
	"ms-appinstaller": true, "ms-cxh": true, "ms-cxh-full": true, "x-apple-systempreferences": false,
	"smb": true, "ftp": true, "jar": true, "res": true, "about": true, "chrome": true,
	"ldap": true, "hcp": true, "its": true, "mk": true, "ms-its": true, "mhtml": true,
}

// RDP target parsed from an rdp:// URI.
type RDP struct {
	Address  string // host[:port]
	Username string
}

// Open opens a link of kind web, rdp or app.
func Open(kind, uri, title string) error {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || u.Scheme == "" {
		return ErrUnsafe
	}
	scheme := strings.ToLower(u.Scheme)
	switch kind {
	case "web":
		if scheme != "http" && scheme != "https" {
			return ErrUnsafe
		}
		return openURL(u.String())
	case "rdp":
		r, err := ParseRDP(uri)
		if err != nil {
			return err
		}
		return openRDP(r, uri, title)
	default:
		if blocked[scheme] || len(scheme) == 1 { // "c:" is a drive letter
			return ErrUnsafe
		}
		return openURL(u.String())
	}
}

// ParseRDP reads rdp://full%20address=s:host:port&username=s:user.
func ParseRDP(uri string) (RDP, error) {
	rest, ok := strings.CutPrefix(uri, "rdp://")
	if !ok {
		return RDP{}, ErrUnsafe
	}
	var r RDP
	for _, part := range strings.Split(rest, "&") {
		k, v, _ := strings.Cut(part, "=")
		key, _ := url.PathUnescape(k)
		val, _ := url.PathUnescape(v)
		val = strings.TrimPrefix(val, "s:")
		switch strings.ToLower(key) {
		case "full address":
			r.Address = val
		case "username":
			r.Username = val
		}
	}
	if r.Address == "" || strings.ContainsAny(r.Address, "\r\n\"") || strings.ContainsAny(r.Username, "\r\n\"") {
		return RDP{}, ErrUnsafe
	}
	return r, nil
}
