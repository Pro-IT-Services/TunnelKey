// Package protect encrypts data at rest with a per-user master key. The key
// itself is protected by the operating system: DPAPI on Windows, the login
// Keychain on macOS and the Secret Service (GNOME Keyring, KWallet) on Linux.
package protect

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Sealer encrypts and decrypts small blobs with AES-256-GCM.
type Sealer struct {
	aead cipher.AEAD
}

// ErrOpen means the data was not sealed with this user's key.
var ErrOpen = errors.New("stored data could not be decrypted")

func newSealer(key []byte) (*Sealer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal returns nonce ‖ ciphertext.
func (s *Sealer) Seal(plain, aad []byte) []byte {
	nonce := make([]byte, s.aead.NonceSize())
	rand.Read(nonce)
	return s.aead.Seal(nonce, nonce, plain, aad)
}

// Open reverses Seal.
func (s *Sealer) Open(sealed, aad []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n+16 {
		return nil, ErrOpen
	}
	out, err := s.aead.Open(nil, sealed[:n], sealed[n:], aad)
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}

// Open loads (or creates) the master key stored next to dir and returns a
// sealer for it. weak reports that no OS key store was available and the key
// is only protected by file permissions.
func OpenSealer(dir string) (s *Sealer, weak bool, err error) {
	key, weak, err := masterKey(dir)
	if err != nil {
		return nil, false, err
	}
	s, err = newSealer(key)
	return s, weak, err
}

func newKey() []byte {
	k := make([]byte, 32)
	rand.Read(k)
	return k
}
