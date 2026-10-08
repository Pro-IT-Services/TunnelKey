package main

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
)

// Setup file (desktop) reference implementation, see docs/provisioning-format.md.
// The web UI encrypts in the browser (web/setupfile.js) so the file password
// never reaches the server; this Go version is only used by the tests to
// cross-check the format.

const (
	setupFileIter    = 600_000
	setupFileIterMin = 100_000
	setupFileIterMax = 10_000_000
)

type setupFile struct {
	Tunnelkey string `json:"tunnelkey"`
	V         int    `json:"v"`
	KDF       struct {
		Alg  string `json:"alg"`
		Iter int    `json:"iter"`
		Salt string `json:"salt"`
	} `json:"kdf"`
	Enc struct {
		Alg string `json:"alg"`
		IV  string `json:"iv"`
	} `json:"enc"`
	Data string `json:"data"`
}

var b64url = base64.RawURLEncoding

func setupFileAAD(iter int, salt, iv string) []byte {
	return []byte(fmt.Sprintf("tunnelkey-setup-file:1:%d:%s:%s", iter, salt, iv))
}

// setupFileAEAD derives the AES-256-GCM key. The password must already be
// NFC-normalised (the browser does that; Go's standard library can't).
func setupFileAEAD(password string, salt []byte, iter int) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iter, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// encryptSetupFile returns the text of a .tunnelkey file for p.
func encryptSetupFile(p payload, password string) ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var z bytes.Buffer
	w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	w.Write(raw)
	w.Close()

	salt, iv := make([]byte, 16), make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	var f setupFile
	f.Tunnelkey, f.V = "setup-file", 1
	f.KDF.Alg, f.KDF.Iter, f.KDF.Salt = "PBKDF2-SHA256", setupFileIter, b64url.EncodeToString(salt)
	f.Enc.Alg, f.Enc.IV = "A256GCM", b64url.EncodeToString(iv)
	aead, err := setupFileAEAD(password, salt, f.KDF.Iter)
	if err != nil {
		return nil, err
	}
	f.Data = b64url.EncodeToString(aead.Seal(nil, iv, z.Bytes(), setupFileAAD(f.KDF.Iter, f.KDF.Salt, f.Enc.IV)))
	out, err := json.MarshalIndent(f, "", "  ")
	return append(out, '\n'), err
}

// decryptSetupFile is the reference reader.
func decryptSetupFile(text []byte, password string) (payload, error) {
	var out payload
	var f setupFile
	if err := json.Unmarshal(text, &f); err != nil || f.Tunnelkey != "setup-file" {
		return out, errors.New("not a Tunnelkey setup file")
	}
	if f.V != 1 {
		return out, fmt.Errorf("unsupported setup file version %d", f.V)
	}
	if f.KDF.Alg != "PBKDF2-SHA256" || f.Enc.Alg != "A256GCM" {
		return out, errors.New("unsupported setup file encryption")
	}
	if f.KDF.Iter < setupFileIterMin || f.KDF.Iter > setupFileIterMax {
		return out, errors.New("iteration count out of range")
	}
	salt, err1 := b64url.Strict().DecodeString(f.KDF.Salt)
	iv, err2 := b64url.Strict().DecodeString(f.Enc.IV)
	data, err3 := b64url.Strict().DecodeString(f.Data)
	if err := errors.Join(err1, err2, err3); err != nil {
		return out, fmt.Errorf("bad base64url field: %w", err)
	}
	if len(salt) != 16 || len(iv) != 12 || len(data) < 16 {
		return out, errors.New("wrong field length")
	}
	aead, err := setupFileAEAD(password, salt, f.KDF.Iter)
	if err != nil {
		return out, err
	}
	plain, err := aead.Open(nil, iv, data, setupFileAAD(f.KDF.Iter, f.KDF.Salt, f.Enc.IV))
	if err != nil {
		return out, errors.New("wrong password, or the file has been modified")
	}
	r, err := zlib.NewReader(bytes.NewReader(plain))
	if err != nil {
		return out, err
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
