//go:build !darwin

package main

import (
	goruntime "runtime"
	"sync"

	"fyne.io/systray"
)

// Windows and Linux tray, via fyne.io/systray. On Windows the tray window and
// its message loop must live on one thread, hence the locked goroutine.

var sysTray struct {
	mu                         sync.Mutex
	status, open, toggle, quit *systray.MenuItem
}

func trayBackendStart(t map[string]string, ready func(), clicked func(int)) {
	go func() {
		goruntime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(trayIcon("idle"))
			systray.SetTitle("Tunnelkey")
			systray.SetTooltip("Tunnelkey")
			systray.SetOnTapped(func() { clicked(trayItemOpen) })

			status := systray.AddMenuItem("Tunnelkey", "")
			status.Disable()
			systray.AddSeparator()
			open := systray.AddMenuItem(t["open"], "")
			toggle := systray.AddMenuItem(t["connect"], "")
			systray.AddSeparator()
			quit := systray.AddMenuItem(t["quit"], "")

			sysTray.mu.Lock()
			sysTray.status, sysTray.open, sysTray.toggle, sysTray.quit = status, open, toggle, quit
			sysTray.mu.Unlock()
			ready()

			go func() {
				for {
					select {
					case <-open.ClickedCh:
						clicked(trayItemOpen)
					case <-toggle.ClickedCh:
						clicked(trayItemToggle)
					case <-quit.ClickedCh:
						clicked(trayItemQuit)
						return
					}
				}
			}()
		}, nil)
	}()
}

func trayBackendStop() { systray.Quit() }

func trayBackendSetIcon(icon []byte) { systray.SetIcon(icon) }

func trayBackendSetStatus(tooltip, line string) {
	systray.SetTooltip(tooltip)
	sysTray.mu.Lock()
	status := sysTray.status
	sysTray.mu.Unlock()
	if status != nil {
		status.SetTitle(line)
	}
}

func trayBackendSetToggle(title string, enabled bool) {
	sysTray.mu.Lock()
	toggle := sysTray.toggle
	sysTray.mu.Unlock()
	if toggle == nil {
		return
	}
	toggle.SetTitle(title)
	if enabled {
		toggle.Enable()
	} else {
		toggle.Disable()
	}
}

func trayBackendSetLabels(open, quit string) {
	sysTray.mu.Lock()
	o, q := sysTray.open, sysTray.quit
	sysTray.mu.Unlock()
	if o != nil {
		o.SetTitle(open)
		q.SetTitle(quit)
	}
}
