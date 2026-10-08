package helper

import (
	"os"
	"strconv"
	"strings"
)

// dnsConfig is what the server pushed, read from openvpn's script environment.
type dnsConfig struct {
	Servers    []string
	Domains    []string
	FullTunnel bool // all traffic goes through the VPN
}

// pushedDNS reads dhcp-option and (OpenVPN 2.6) dns-option variables.
func pushedDNS(getenv func(string) string) dnsConfig {
	var c dnsConfig
	for i := 1; ; i++ {
		v := getenv("foreign_option_" + strconv.Itoa(i))
		if v == "" {
			break
		}
		f := strings.Fields(v)
		if len(f) < 3 || f[0] != "dhcp-option" {
			continue
		}
		switch strings.ToUpper(f[1]) {
		case "DNS", "DNS6":
			c.Servers = append(c.Servers, f[2])
		case "DOMAIN", "DOMAIN-SEARCH", "ADAPTER_DOMAIN_SUFFIX":
			c.Domains = append(c.Domains, f[2])
		}
	}
	for n := 0; n < 8; n++ {
		for m := 1; m <= 8; m++ {
			if v := getenv("dns_server_" + strconv.Itoa(n) + "_address_" + strconv.Itoa(m)); v != "" {
				c.Servers = append(c.Servers, v)
			}
		}
	}
	for i := 1; i <= 16; i++ {
		if v := getenv("dns_search_domain_" + strconv.Itoa(i)); v != "" {
			c.Domains = append(c.Domains, v)
		}
	}
	// redirect-gateway def1 shows up as 0.0.0.0/128.0.0.0 routes.
	for i := 1; i <= 512; i++ {
		net := getenv("route_network_" + strconv.Itoa(i))
		if net == "" {
			break
		}
		if net == "0.0.0.0" {
			c.FullTunnel = true
		}
	}
	if getenv("redirect_gateway") != "" && getenv("redirect_gateway") != "0" {
		c.FullTunnel = true
	}
	return c
}

// RunDNSHook is called by openvpn as the up/down script (argv: dns-hook).
func RunDNSHook() error {
	return applyDNS(os.Getenv("script_type"), os.Getenv("dev"), pushedDNS(os.Getenv))
}
