package protect

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var entropy = []byte("Tunnelkey master key v1")

// masterKey keeps the key in a DPAPI blob bound to the Windows user.
func masterKey(dir string) ([]byte, bool, error) {
	path := filepath.Join(dir, "master.key")
	if blob, err := os.ReadFile(path); err == nil {
		key, err := unprotect(blob)
		return key, false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	key := newKey()
	blob, err := protect(key)
	if err != nil {
		return nil, false, err
	}
	return key, false, os.WriteFile(path, blob, 0o600)
}

func blobOf(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blobOf(plain), nil, blobOf(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unprotect(blob []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blobOf(blob), nil, blobOf(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, ErrOpen
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
