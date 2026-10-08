package main

import "github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"

// No tray on macOS yet: the tray library and Wails both need the main
// thread there. Closing the window quits the app as usual.
const trayAvailable = false

func (a *App) startTray()                  {}
func (a *App) stopTray()                   {}
func (a *App) trayUpdate(ipc.Status, bool) {}
func (a *App) trayRelabel()                {}
