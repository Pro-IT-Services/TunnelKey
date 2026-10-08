package helper

import (
	"fmt"
	"os/exec"
)

// applyDNS hands the pushed DNS servers to systemd-resolved for the tunnel
// interface. Without systemd-resolved the system resolver is left alone.
func applyDNS(scriptType, dev string, c dnsConfig) error {
	resolvectl, err := exec.LookPath("resolvectl")
	if err != nil || dev == "" {
		return nil
	}
	run := func(args ...string) error {
		out, err := exec.Command(resolvectl, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("resolvectl %v: %v: %s", args, err, out)
		}
		return nil
	}
	if scriptType == "down" {
		return run("revert", dev)
	}
	if scriptType != "up" || len(c.Servers) == 0 {
		return nil
	}
	if err := run(append([]string{"dns", dev}, c.Servers...)...); err != nil {
		return err
	}
	domains := append([]string(nil), c.Domains...)
	if c.FullTunnel {
		domains = append(domains, "~.") // route every lookup through the tunnel
		run("default-route", dev, "true")
	}
	if len(domains) > 0 {
		return run(append([]string{"domain", dev}, domains...)...)
	}
	return nil
}
