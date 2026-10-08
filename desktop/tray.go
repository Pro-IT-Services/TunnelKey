//go:build !darwin

package main

import (
	_ "embed"
	goruntime "runtime"
	"sync"
	"time"

	"fyne.io/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"
)

// The tray icon shows the tunnel state and keeps the app reachable while its
// window is hidden (closing the window hides it; Quit ends the app).
const trayAvailable = true

var (
	//go:embed trayicons/idle.ico
	trayIdleICO []byte
	//go:embed trayicons/busy.ico
	trayBusyICO []byte
	//go:embed trayicons/secure.ico
	traySecureICO []byte
	//go:embed trayicons/idle.png
	trayIdlePNG []byte
	//go:embed trayicons/busy.png
	trayBusyPNG []byte
	//go:embed trayicons/secure.png
	traySecurePNG []byte
)

type trayState struct {
	mu     sync.Mutex
	ready  bool
	status *systray.MenuItem
	open   *systray.MenuItem
	toggle *systray.MenuItem
	quit   *systray.MenuItem
	last   ipc.Status
	helper bool
	icon   string
	texts  map[string]string
}

var tray trayState

func trayIcon(name string) []byte {
	ico := goruntime.GOOS == "windows"
	switch name {
	case "busy":
		if ico {
			return trayBusyICO
		}
		return trayBusyPNG
	case "secure":
		if ico {
			return traySecureICO
		}
		return traySecurePNG
	default:
		if ico {
			return trayIdleICO
		}
		return trayIdlePNG
	}
}

// startTray runs the tray on its own OS thread: on Windows the tray window
// and its message loop must live on the same thread.
func (a *App) startTray() {
	tray.texts = trayTexts(a.trayLanguage())
	go func() {
		goruntime.LockOSThread()
		systray.Run(func() { a.trayReady() }, nil)
	}()
}

func (a *App) stopTray() {
	systray.Quit()
}

// trayRelabel switches the menu language after a settings change.
func (a *App) trayRelabel() {
	tray.mu.Lock()
	tray.texts = trayTexts(a.trayLanguage())
	ready := tray.ready
	open, quit := tray.open, tray.quit
	last, helper := tray.last, tray.helper
	t := tray.texts
	tray.mu.Unlock()
	if !ready {
		return
	}
	open.SetTitle(t["open"])
	quit.SetTitle(t["quit"])
	a.trayUpdate(last, helper)
}

func (a *App) trayReady() {
	t := tray.texts
	systray.SetIcon(trayIcon("idle"))
	systray.SetTitle("Tunnelkey")
	systray.SetTooltip("Tunnelkey")
	systray.SetOnTapped(a.showWindow)

	status := systray.AddMenuItem("Tunnelkey", "")
	status.Disable()
	systray.AddSeparator()
	open := systray.AddMenuItem(t["open"], "")
	toggle := systray.AddMenuItem(t["connect"], "")
	systray.AddSeparator()
	quit := systray.AddMenuItem(t["quit"], "")

	tray.mu.Lock()
	tray.status, tray.open, tray.toggle, tray.quit = status, open, toggle, quit
	tray.ready = true
	last, helper := tray.last, tray.helper
	tray.mu.Unlock()
	a.trayUpdate(last, helper)

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				a.showWindow()
			case <-toggle.ClickedCh:
				a.trayToggle()
			case <-quit.ClickedCh:
				a.quitFromTray()
				return
			}
		}
	}()
}

// trayUpdate reflects the tunnel state in the icon, tooltip and menu.
func (a *App) trayUpdate(st ipc.Status, helperUp bool) {
	tray.mu.Lock()
	tray.last, tray.helper = st, helperUp
	if !tray.ready {
		tray.mu.Unlock()
		return
	}
	t := tray.texts
	phase := st.Phase
	if phase == "" {
		phase = ipc.Disconnected
	}
	line := t[string(phase)]
	if !helperUp {
		line = t["helper_down"]
	} else if st.ProfileName != "" && phase != ipc.Disconnected {
		line += " · " + st.ProfileName
	}
	icon := "idle"
	switch phase {
	case ipc.Connected:
		icon = "secure"
	case ipc.Connecting, ipc.Reconnecting, ipc.Disconnecting:
		icon = "busy"
	}
	changedIcon := icon != tray.icon
	tray.icon = icon
	status, toggle := tray.status, tray.toggle
	tray.mu.Unlock()

	if changedIcon {
		systray.SetIcon(trayIcon(icon))
	}
	systray.SetTooltip("Tunnelkey: " + line)
	status.SetTitle(line)
	switch phase {
	case ipc.Connected:
		toggle.SetTitle(t["disconnect"])
	case ipc.Connecting, ipc.Reconnecting, ipc.Disconnecting:
		toggle.SetTitle(t["cancel"])
	default:
		toggle.SetTitle(t["connect"])
	}
	if helperUp {
		toggle.Enable()
	} else {
		toggle.Disable()
	}
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
	// Bring it in front of other windows without keeping it on top.
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wruntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// trayToggle disconnects, or asks the UI to connect: the UI knows the
// selected profile and opens the sign-in form when a password or code is
// needed.
func (a *App) trayToggle() {
	a.mu.Lock()
	phase := a.status.Phase
	a.mu.Unlock()
	switch phase {
	case ipc.Connected, ipc.Connecting, ipc.Reconnecting, ipc.Disconnecting:
		a.Disconnect()
	default:
		a.showWindow()
		wruntime.EventsEmit(a.ctx, "tray-connect")
	}
}

// quitFromTray disconnects (a tunnel without its app would be invisible)
// and ends the app.
func (a *App) quitFromTray() {
	a.mu.Lock()
	phase := a.status.Phase
	a.mu.Unlock()
	if phase != ipc.Disconnected && phase != ipc.Failed {
		a.Disconnect()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			a.mu.Lock()
			p := a.status.Phase
			a.mu.Unlock()
			if p == ipc.Disconnected || p == ipc.Failed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	systray.Quit()
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}
