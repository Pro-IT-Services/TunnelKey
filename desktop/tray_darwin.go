package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "tray_darwin.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// macOS menu bar backend (tray_darwin.m). Click and ready callbacks arrive on
// the main thread and are handed to goroutines so AppKit isn't blocked.

var darwinTray struct {
	mu      sync.Mutex
	ready   func()
	clicked func(int)
}

func cstr(s string) *C.char { return C.CString(s) }

func trayBackendStart(t map[string]string, ready func(), clicked func(int)) {
	darwinTray.mu.Lock()
	darwinTray.ready, darwinTray.clicked = ready, clicked
	darwinTray.mu.Unlock()
	o, c, q := cstr(t["open"]), cstr(t["connect"]), cstr(t["quit"])
	defer C.free(unsafe.Pointer(o))
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(q))
	C.tkTrayStart(o, c, q)
}

//export tunnelkeyTrayReady
func tunnelkeyTrayReady() {
	darwinTray.mu.Lock()
	ready := darwinTray.ready
	darwinTray.mu.Unlock()
	if ready != nil {
		go ready()
	}
}

//export tunnelkeyTrayClicked
func tunnelkeyTrayClicked(item C.int) {
	darwinTray.mu.Lock()
	clicked := darwinTray.clicked
	darwinTray.mu.Unlock()
	if clicked != nil {
		go clicked(int(item))
	}
}

func trayBackendStop() { C.tkTrayStop() }

func trayBackendSetIcon(png []byte) {
	if len(png) == 0 {
		return
	}
	C.tkTraySetIcon(unsafe.Pointer(&png[0]), C.int(len(png)))
}

func trayBackendSetStatus(tooltip, line string) {
	t, l := cstr(tooltip), cstr(line)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(l))
	C.tkTraySetStatus(t, l)
}

func trayBackendSetToggle(title string, enabled bool) {
	t := cstr(title)
	defer C.free(unsafe.Pointer(t))
	e := C.int(0)
	if enabled {
		e = 1
	}
	C.tkTraySetToggle(t, e)
}

func trayBackendSetLabels(open, quit string) {
	o, q := cstr(open), cstr(quit)
	defer C.free(unsafe.Pointer(o))
	defer C.free(unsafe.Pointer(q))
	C.tkTraySetLabels(o, q)
}
