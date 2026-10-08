package helper

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var unsafeSizeofSA = unsafe.Sizeof(windows.SecurityAttributes{})
