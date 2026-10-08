// Package vault stores the provisioned password and TOTP secret, like the
// phone apps' Vault:
//
//   - Hello: sealed with the user's master key (DPAPI); the app only opens it
//     after Windows Hello confirms the user.
//   - PIN: AES-256-GCM with a key derived from the 8-digit PIN
//     (PBKDF2-SHA256), sealed again with the master key so the blob can't be
//     brute-forced on another computer. Wrong PINs cause growing delays; the
//     10th erases it.
//   - None: master key only (allowed when nothing secret is stored).
//
// The whole vault file, including the failure counter, is sealed so it can't
// be reset by editing it.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kalipsers/TunnelKey/desktop/internal/protect"
)

// Method protecting the vault.
type Method string

const (
	None  Method = "none"
	Pin   Method = "pin"
	Hello Method = "hello"
)

const (
	MaxAttempts      = 10
	pbkdf2Iterations = 310_000
)

// Secrets of the provisioned configuration.
type Secrets struct {
	Password   string `json:"password,omitempty"`
	TOTPSecret string `json:"totpSecret,omitempty"`
}

// PinResult of an unlock attempt.
type PinResult struct {
	Secrets      *Secrets
	AttemptsLeft int
	LockedUntil  time.Time
	Wiped        bool // too many wrong PINs: the configuration was erased
}

var (
	ErrNoVault  = errors.New("no stored secrets")
	ErrWrongPin = errors.New("wrong PIN")
)

type state struct {
	Method      Method `json:"method"`
	Blob        []byte `json:"blob"`
	Salt        []byte `json:"salt,omitempty"`
	Failures    int    `json:"failures"`
	LockedUntil int64  `json:"lockedUntil"` // unix ms
}

// Vault is safe for concurrent use.
type Vault struct {
	mu     sync.Mutex
	path   string
	sealer *protect.Sealer
	now    func() time.Time
}

// Open a vault in dir.
func Open(dir string, sealer *protect.Sealer) *Vault {
	return &Vault{path: filepath.Join(dir, "vault.bin"), sealer: sealer, now: time.Now}
}

func (v *Vault) load() (*state, error) {
	sealed, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoVault
	}
	if err != nil {
		return nil, err
	}
	plain, err := v.sealer.Open(sealed, []byte("vault"))
	if err != nil {
		return nil, err
	}
	var st state
	return &st, json.Unmarshal(plain, &st)
}

func (v *Vault) save(st *state) error {
	plain, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, v.sealer.Seal(plain, []byte("vault")), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

// Method returns how the vault is protected ("" when there is none).
func (v *Vault) Method() Method {
	v.mu.Lock()
	defer v.mu.Unlock()
	st, err := v.load()
	if err != nil {
		return ""
	}
	return st.Method
}

// Store replaces the vault. pin is only used with Pin.
func (v *Vault) Store(m Method, pin string, s Secrets) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	plain, err := json.Marshal(s)
	if err != nil {
		return err
	}
	st := &state{Method: m}
	switch m {
	case Pin:
		st.Salt = make([]byte, 16)
		rand.Read(st.Salt)
		aead, err := pinAEAD(pin, st.Salt)
		if err != nil {
			return err
		}
		nonce := make([]byte, aead.NonceSize())
		rand.Read(nonce)
		st.Blob = aead.Seal(nonce, nonce, plain, nil)
	case None, Hello:
		st.Blob = plain // the whole state is sealed with the master key
	default:
		return errors.New("unknown lock method")
	}
	return v.save(st)
}

// Open returns the secrets of a None or Hello vault. For Hello the caller
// must have verified the user first.
func (v *Vault) Open() (*Secrets, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	st, err := v.load()
	if err != nil {
		return nil, err
	}
	if st.Method == Pin {
		return nil, errors.New("this vault needs the PIN")
	}
	var s Secrets
	return &s, json.Unmarshal(st.Blob, &s)
}

// UnlockWithPin tries a PIN.
func (v *Vault) UnlockWithPin(pin string) (PinResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	st, err := v.load()
	if err != nil {
		return PinResult{}, err
	}
	now := v.now()
	if until := time.UnixMilli(st.LockedUntil); now.Before(until) {
		return PinResult{AttemptsLeft: MaxAttempts - st.Failures, LockedUntil: until}, ErrWrongPin
	}
	aead, err := pinAEAD(pin, st.Salt)
	if err != nil {
		return PinResult{}, err
	}
	n := aead.NonceSize()
	if len(st.Blob) > n {
		if plain, err := aead.Open(nil, st.Blob[:n], st.Blob[n:], nil); err == nil {
			st.Failures, st.LockedUntil = 0, 0
			v.save(st)
			var s Secrets
			if err := json.Unmarshal(plain, &s); err != nil {
				return PinResult{}, err
			}
			return PinResult{Secrets: &s}, nil
		}
	}
	st.Failures++
	if st.Failures >= MaxAttempts {
		os.Remove(v.path)
		return PinResult{Wiped: true}, ErrWrongPin
	}
	until := now.Add(DelayAfter(st.Failures))
	st.LockedUntil = until.UnixMilli()
	if err := v.save(st); err != nil {
		return PinResult{}, err
	}
	return PinResult{AttemptsLeft: MaxAttempts - st.Failures, LockedUntil: until}, ErrWrongPin
}

// Status reports failures and lock-out for the PIN screen.
func (v *Vault) Status() (failures int, lockedUntil time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	st, err := v.load()
	if err != nil {
		return 0, time.Time{}
	}
	return st.Failures, time.UnixMilli(st.LockedUntil)
}

// Wipe erases the vault.
func (v *Vault) Wipe() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	err := os.Remove(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// DelayAfter matches the phone apps: no delay for the first 4 mistakes, then
// 30 s, 1 min, 5 min, 15 min, 1 h.
func DelayAfter(failures int) time.Duration {
	switch {
	case failures < 5:
		return 0
	case failures == 5:
		return 30 * time.Second
	case failures == 6:
		return time.Minute
	case failures == 7:
		return 5 * time.Minute
	case failures == 8:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func pinAEAD(pin string, salt []byte) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, pin, salt, pbkdf2Iterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
