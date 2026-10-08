package main

import (
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"

	"github.com/kalipsers/TunnelKey/desktop/internal/helper"
	"github.com/kalipsers/TunnelKey/desktop/internal/ipc"
)

// ServiceName as registered by the installer.
const ServiceName = "TunnelkeyHelper"

type service struct{}

func (service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	log.Printf("tunnelkey-helper %s starting", version)
	e := helper.New()
	l, err := ipc.Listen()
	if err != nil {
		log.Printf("listen: %v", err)
		return true, 1
	}
	go e.Serve(l)
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			e.Shutdown()
			l.Close()
			return false, 0
		}
	}
	return false, 0
}

func runAsService() bool {
	is, err := svc.IsWindowsService()
	if err != nil || !is {
		return false
	}
	logToFile()
	if err := svc.Run(ServiceName, service{}); err != nil {
		log.Printf("service: %v", err)
	}
	return true
}

// logToFile keeps the service's own log in %ProgramData%\Tunnelkey (a
// service has no console); it is truncated when it grows past 1 MB.
func logToFile() {
	dir := filepath.Join(os.Getenv("ProgramData"), "Tunnelkey")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	path := filepath.Join(dir, "helper.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		flags |= os.O_TRUNC
	}
	if f, err := os.OpenFile(path, flags, 0o644); err == nil {
		log.SetOutput(f)
	}
}
