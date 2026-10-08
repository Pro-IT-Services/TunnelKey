// Command Tunnelkey is the desktop app: an OpenVPN client for servers that
// sign in with a password plus a one-time code. The tunnel itself is run by
// the privileged tunnelkey-helper service.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     windowTitle,
		Width:     440,
		Height:    760,
		MinWidth:  380,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 20, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.proitservices.tunnelkey",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) { app.onSecondInstance(d.Args) },
		},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Mac: &mac.Options{
			About:      &mac.AboutInfo{Title: "Tunnelkey", Message: "OpenVPN client with two-factor sign-in.\nProIT services", Icon: icon},
			OnFileOpen: app.onFileOpen,
		},
		Linux: &linux.Options{
			Icon:        icon,
			ProgramName: "tunnelkey",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
