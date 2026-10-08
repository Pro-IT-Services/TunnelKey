// Command tunnelkey-helper is the privileged half of Tunnelkey for Windows,
// macOS and Linux. It runs as a system service (Windows service, launchd
// daemon, systemd unit) and starts openvpn on behalf of the Tunnelkey app.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/helper"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"
)

var version = "dev"

func main() {
	helper.Version = version
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "dns-hook":
			// Called by openvpn as its up/down script.
			if err := helper.RunDNSHook(); err != nil {
				fmt.Fprintln(os.Stderr, "tunnelkey dns-hook:", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Println(version)
			return
		}
	}
	if runAsService() {
		return
	}
	// Foreground mode (launchd, systemd, or debugging).
	e := helper.New()
	l, err := ipc.Listen()
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		e.Shutdown()
		l.Close()
	}()
	log.Printf("tunnelkey-helper %s listening", version)
	if err := e.Serve(l); err != nil {
		log.Printf("stopped: %v", err)
	}
}
