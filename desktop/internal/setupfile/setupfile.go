// Package setupfile reads password-encrypted setup files (.tunnelkey), see
// docs/provisioning-format.md, "Setup file format (v1) — desktop".
package setupfile

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const (
	// DefaultIterations is what writers use.
	DefaultIterations = 600_000
	minIterations     = 100_000
	maxIterations     = 10_000_000
	// MaxFileBytes bounds what is read from disk.
	MaxFileBytes = 1 << 20
	maxPlain     = 512 * 1024
)

var (
	ErrNotSetupFile  = errors.New("this is not a Tunnelkey setup file")
	ErrNewerVersion  = errors.New("this setup file needs a newer version of Tunnelkey")
	ErrWrongPassword = errors.New("wrong password, or the file was changed")
	ErrDamaged       = errors.New("the setup file is damaged")
)

// TOTP settings provisioned with the configuration.
type TOTP struct {
	Secret    string `json:"s"`
	Digits    int    `json:"d"`
	Period    int    `json:"p"`
	Algorithm string `json:"a"`
}

// Link shown under the configuration.
type Link struct {
	Title string `json:"t"`
	Kind  string `json:"k"` // web, rdp, app
	URI   string `json:"u"`
}

// Payload is the provisioned configuration (format section 1).
type Payload struct {
	Version      int    `json:"v"`
	Name         string `json:"n"`
	OVPN         string `json:"o"`
	Username     string `json:"u,omitempty"`
	Password     string `json:"p,omitempty"`
	TOTP         *TOTP  `json:"t,omitempty"`
	ManualCode   bool   `json:"f,omitempty"`
	CodePosition string `json:"c,omitempty"` // "a" after (default), "b" before
	Links        []Link `json:"l,omitempty"`
}

type kdfParams struct {
	Alg  string `json:"alg"`
	Iter int    `json:"iter"`
	Salt string `json:"salt"`
}

type encParams struct {
	Alg string `json:"alg"`
	IV  string `json:"iv"`
}

type envelope struct {
	Magic   string    `json:"tunnelkey"`
	Version int       `json:"v"`
	KDF     kdfParams `json:"kdf"`
	Enc     encParams `json:"enc"`
	Data    string    `json:"data"`
}

var b64 = base64.RawURLEncoding

// Decrypt opens a setup file with its password.
func Decrypt(file []byte, password string) (*Payload, error) {
	if len(file) > MaxFileBytes {
		return nil, ErrNotSetupFile
	}
	file = bytes.TrimPrefix(file, []byte("\xef\xbb\xbf"))
	var env envelope
	if err := json.Unmarshal(file, &env); err != nil || env.Magic != "setup-file" {
		return nil, ErrNotSetupFile
	}
	if env.Version != 1 {
		if env.Version > 1 {
			return nil, ErrNewerVersion
		}
		return nil, ErrNotSetupFile
	}
	if env.KDF.Alg != "PBKDF2-SHA256" || env.Enc.Alg != "A256GCM" {
		return nil, ErrNewerVersion
	}
	if env.KDF.Iter < minIterations || env.KDF.Iter > maxIterations {
		return nil, ErrDamaged
	}
	salt, err1 := b64.DecodeString(env.KDF.Salt)
	iv, err2 := b64.DecodeString(env.Enc.IV)
	data, err3 := b64.DecodeString(env.Data)
	if err1 != nil || err2 != nil || err3 != nil || len(salt) < 16 || len(iv) != 12 || len(data) < 16 {
		return nil, ErrDamaged
	}
	gcm, err := newGCM(password, salt, env.KDF.Iter)
	if err != nil {
		return nil, err
	}
	compressed, err := gcm.Open(nil, iv, data, aad(env.KDF.Iter, env.KDF.Salt, env.Enc.IV))
	if err != nil {
		return nil, ErrWrongPassword
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, ErrDamaged
	}
	plain, err := io.ReadAll(io.LimitReader(zr, maxPlain+1))
	if err != nil || len(plain) > maxPlain {
		return nil, ErrDamaged
	}
	var p Payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrDamaged
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Encrypt writes a setup file. The app never creates them; this exists for
// tests and tooling.
func Encrypt(p *Payload, password string) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write(plain)
	zw.Close()

	salt := make([]byte, 16)
	iv := make([]byte, 12)
	rand.Read(salt)
	rand.Read(iv)
	gcm, err := newGCM(password, salt, DefaultIterations)
	if err != nil {
		return nil, err
	}
	env := envelope{
		Magic:   "setup-file",
		Version: 1,
		KDF:     kdfParams{Alg: "PBKDF2-SHA256", Iter: DefaultIterations, Salt: b64.EncodeToString(salt)},
		Enc:     encParams{Alg: "A256GCM", IV: b64.EncodeToString(iv)},
	}
	env.Data = b64.EncodeToString(gcm.Seal(nil, iv, zbuf.Bytes(), aad(env.KDF.Iter, env.KDF.Salt, env.Enc.IV)))
	out, err := json.MarshalIndent(env, "", "  ")
	return append(out, '\n'), err
}

func newGCM(password string, salt []byte, iter int) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, norm.NFC.String(password), salt, iter, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func aad(iter int, salt, iv string) []byte {
	return []byte(fmt.Sprintf("tunnelkey-setup-file:1:%d:%s:%s", iter, salt, iv))
}

func (p *Payload) validate() error {
	if p.Version != 1 {
		if p.Version > 1 {
			return ErrNewerVersion
		}
		return ErrDamaged
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.OVPN) == "" {
		return ErrDamaged
	}
	if t := p.TOTP; t != nil {
		if t.Digits == 0 {
			t.Digits = 6
		}
		if t.Period == 0 {
			t.Period = 30
		}
		if t.Algorithm == "" {
			t.Algorithm = "SHA1"
		}
		if (t.Digits != 6 && t.Digits != 8) || t.Period < 10 || t.Period > 300 || t.Secret == "" {
			return ErrDamaged
		}
	}
	return nil
}

// CodeAfter reports whether the one-time code goes after the password.
func (p *Payload) CodeAfter() bool { return p.CodePosition != "b" }
