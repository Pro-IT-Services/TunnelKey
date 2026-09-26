package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TOTPConfig struct {
	Secret    string `json:"secret"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Algorithm string `json:"algorithm"`
}

// normalize validates the config and canonicalises the secret (upper-case
// base32, no spaces or padding).
func (t *TOTPConfig) normalize() error {
	t.Secret = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "=", "").Replace(t.Secret))
	if t.Secret == "" {
		return errors.New("TOTP secret is empty")
	}
	if _, err := decodeBase32(t.Secret); err != nil {
		return errors.New("TOTP secret is not valid base32")
	}
	if t.Digits == 0 {
		t.Digits = 6
	}
	if t.Digits != 6 && t.Digits != 8 {
		return errors.New("TOTP digits must be 6 or 8")
	}
	if t.Period == 0 {
		t.Period = 30
	}
	if t.Period < 10 || t.Period > 300 {
		return errors.New("TOTP period must be 10–300 seconds")
	}
	t.Algorithm = strings.ToUpper(t.Algorithm)
	if t.Algorithm == "" {
		t.Algorithm = "SHA1"
	}
	switch t.Algorithm {
	case "SHA1", "SHA256", "SHA512":
	default:
		return errors.New("TOTP algorithm must be SHA1, SHA256 or SHA512")
	}
	return nil
}

func decodeBase32(s string) ([]byte, error) {
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

// code returns the TOTP value for time t (RFC 6238).
func (t TOTPConfig) code(at time.Time) (string, error) {
	key, err := decodeBase32(t.Secret)
	if err != nil {
		return "", err
	}
	var h func() hash.Hash
	switch t.Algorithm {
	case "SHA256":
		h = sha256.New
	case "SHA512":
		h = sha512.New
	default:
		h = sha1.New
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix())/uint64(t.Period))
	mac := hmac.New(h, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < t.Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", t.Digits, v%mod), nil
}

// parseOTPAuthURI reads an otpauth://totp/... URI as exported by most
// authenticator setups.
func parseOTPAuthURI(raw string) (TOTPConfig, error) {
	var t TOTPConfig
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "otpauth" {
		return t, errors.New("not an otpauth:// URI")
	}
	if u.Host != "totp" {
		return t, errors.New("only TOTP (time-based) codes are supported")
	}
	q := u.Query()
	t.Secret = q.Get("secret")
	t.Algorithm = q.Get("algorithm")
	if d := q.Get("digits"); d != "" {
		t.Digits, _ = strconv.Atoi(d)
	}
	if p := q.Get("period"); p != "" {
		t.Period, _ = strconv.Atoi(p)
	}
	return t, t.normalize()
}
