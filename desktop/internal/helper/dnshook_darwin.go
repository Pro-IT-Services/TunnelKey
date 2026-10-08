package helper

import (
	"fmt"
	"os/exec"
	"strings"
)

// applyDNS publishes a resolver configuration for the tunnel through
// configd (scutil), which mDNSResponder picks up.
func applyDNS(scriptType, dev string, c dnsConfig) error {
	key := "State:/Network/Service/tunnelkey-" + dev + "/DNS"
	var script strings.Builder
	switch scriptType {
	case "down":
		fmt.Fprintf(&script, "remove %s\n", key)
	case "up":
		if len(c.Servers) == 0 {
			return nil
		}
		script.WriteString("d.init\n")
		fmt.Fprintf(&script, "d.add ServerAddresses * %s\n", strings.Join(c.Servers, " "))
		if len(c.Domains) > 0 {
			fmt.Fprintf(&script, "d.add SearchDomains * %s\n", strings.Join(c.Domains, " "))
		}
		match := c.Domains
		if c.FullTunnel {
			match = append([]string{`""`}, match...) // empty domain: every lookup
		}
		if len(match) > 0 {
			fmt.Fprintf(&script, "d.add SupplementalMatchDomains * %s\n", strings.Join(match, " "))
		}
		fmt.Fprintf(&script, "set %s\n", key)
	default:
		return nil
	}
	cmd := exec.Command("/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader(script.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("scutil: %v: %s", err, out)
	}
	return nil
}
