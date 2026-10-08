//go:build !windows

package protect

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "Tunnelkey"
	keyringUser    = "master-key"
)

// masterKey keeps the key in the OS key store. Without one (e.g. a Linux
// session with no Secret Service) it falls back to a 0600 file.
func masterKey(dir string) ([]byte, bool, error) {
	if v, err := keyring.Get(keyringService, keyringUser); err == nil {
		key, err := base64.StdEncoding.DecodeString(v)
		if err == nil && len(key) == 32 {
			return key, false, nil
		}
	}
	path := filepath.Join(dir, "master.key")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		// Move a fallback key into the key store once one is available.
		if keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(b)) == nil {
			os.Remove(path)
			return b, false, nil
		}
		return b, true, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	key := newKey()
	if keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(key)) == nil {
		return key, false, nil
	}
	return key, true, os.WriteFile(path, key, 0o600)
}
