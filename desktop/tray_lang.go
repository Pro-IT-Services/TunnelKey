package main

import (
	"os"
	"strings"
)

// trayLanguage: the app setting, else the system language (Slovak or English),
// matching the window's choice.
func (a *App) trayLanguage() string {
	if a.store != nil {
		if l := a.store.Settings().Language; l != "" {
			return l
		}
	}
	for _, l := range systemLanguages() {
		if strings.HasPrefix(strings.ToLower(l), "sk") {
			return "sk"
		}
		if l != "" {
			return "en"
		}
	}
	return "en"
}

func envLanguages() []string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return []string{v}
		}
	}
	return nil
}

func trayTexts(lang string) map[string]string {
	if lang == "sk" {
		return map[string]string{
			"open": "Otvoriť Tunnelkey", "connect": "Pripojiť", "disconnect": "Odpojiť", "cancel": "Zrušiť",
			"quit": "Ukončiť Tunnelkey", "helper_down": "Pomocná služba nebeží",
			"disconnected": "Nepripojené", "connecting": "Pripája sa…", "connected": "Chránené",
			"reconnecting": "Znova sa pripája…", "disconnecting": "Odpája sa…", "failed": "Pripojenie zlyhalo",
		}
	}
	return map[string]string{
		"open": "Open Tunnelkey", "connect": "Connect", "disconnect": "Disconnect", "cancel": "Cancel",
		"quit": "Quit Tunnelkey", "helper_down": "Helper service not running",
		"disconnected": "Not connected", "connecting": "Connecting…", "connected": "Protected",
		"reconnecting": "Reconnecting…", "disconnecting": "Disconnecting…", "failed": "Connection failed",
	}
}
