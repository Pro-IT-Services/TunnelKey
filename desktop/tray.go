package main

import (
	_ "embed"
	goruntime "runtime"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"
)

// The tray (menu bar on macOS) icon shows the tunnel state and keeps the app
// reachable while its window is hidden: closing the window hides it, Quit in
// the tray menu disconnects and ends the app. Platform backends:
// tray_systray.go (Windows, Linux) and tray_darwin.go (macOS).

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

// Menu item ids shared with the backends.
const (
	trayItemOpen = iota + 1
	trayItemToggle
	trayItemQuit
)

type trayState struct {
	mu     sync.Mutex
	ready  bool
	last   ipc.Status
	helper bool
	icon   string
	texts  map[string]string
}

var tray trayState

// trayIcon returns the icon bytes for a state: .ico on Windows, PNG elsewhere.
func trayIcon(name string) []byte {
	ico := goruntime.GOOS == "windows"
	pick := func(i, p []byte) []byte {
		if ico {
			return i
		}
		return p
	}
	switch name {
	case "busy":
		return pick(trayBusyICO, trayBusyPNG)
	case "secure":
		return pick(traySecureICO, traySecurePNG)
	default:
		return pick(trayIdleICO, trayIdlePNG)
	}
}

func (a *App) startTray() {
	tray.mu.Lock()
	tray.texts = trayTexts(a.trayLanguage())
	t := tray.texts
	tray.mu.Unlock()
	trayBackendStart(t, func() {
		tray.mu.Lock()
		tray.ready = true
		last, helper := tray.last, tray.helper
		tray.mu.Unlock()
		a.trayUpdate(last, helper)
	}, a.trayClicked)
}

func (a *App) stopTray() { trayBackendStop() }

func (a *App) trayClicked(item int) {
	switch item {
	case trayItemOpen:
		a.showWindow()
	case trayItemToggle:
		a.trayToggle()
	case trayItemQuit:
		a.quitFromTray()
	}
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
	toggle := t["connect"]
	switch phase {
	case ipc.Connected:
		icon, toggle = "secure", t["disconnect"]
	case ipc.Connecting, ipc.Reconnecting, ipc.Disconnecting:
		icon, toggle = "busy", t["cancel"]
	}
	changedIcon := icon != tray.icon
	tray.icon = icon
	tray.mu.Unlock()

	if changedIcon {
		trayBackendSetIcon(trayIcon(icon))
	}
	trayBackendSetStatus("Tunnelkey: "+line, line)
	trayBackendSetToggle(toggle, helperUp)
}

// trayRelabel switches the menu language after a settings change.
func (a *App) trayRelabel() {
	tray.mu.Lock()
	tray.texts = trayTexts(a.trayLanguage())
	t, ready, last, helper := tray.texts, tray.ready, tray.last, tray.helper
	tray.mu.Unlock()
	if !ready {
		return
	}
	trayBackendSetLabels(t["open"], t["quit"])
	a.trayUpdate(last, helper)
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
	trayBackendStop()
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}
