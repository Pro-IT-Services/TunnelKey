package main

import (
	"log"

	"golang.org/x/sys/windows/svc"

	"github.com/kalipsers/TunnelKey/desktop/internal/helper"
	"github.com/kalipsers/TunnelKey/desktop/internal/ipc"
)

// ServiceName as registered by the installer.
const ServiceName = "TunnelkeyHelper"

type service struct{}

func (service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
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
	if err := svc.Run(ServiceName, service{}); err != nil {
		log.Printf("service: %v", err)
	}
	return true
}
