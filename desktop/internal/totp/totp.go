// Package totp implements RFC 6238 time-based one-time passwords.
package totp

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
	"strings"
	"time"
)

// Generator produces codes for one secret.
type Generator struct {
	Secret    []byte
	Digits    int
	Period    int
	Algorithm string // SHA1, SHA256, SHA512
}

// New decodes a base32 secret.
func New(secret string, digits, period int, algorithm string) (*Generator, error) {
	s := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "=", "").Replace(secret))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil || len(key) == 0 {
		return nil, errors.New("invalid TOTP secret")
	}
	if digits == 0 {
		digits = 6
	}
	if period == 0 {
		period = 30
	}
	return &Generator{Secret: key, Digits: digits, Period: period, Algorithm: strings.ToUpper(algorithm)}, nil
}

// Code returns the code valid at t.
func (g *Generator) Code(t time.Time) string {
	var h func() hash.Hash
	switch g.Algorithm {
	case "SHA256":
		h = sha256.New
	case "SHA512":
		h = sha512.New
	default:
		h = sha1.New
	}
	mac := hmac.New(h, g.Secret)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(t.Unix())/uint64(g.Period))
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < g.Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", g.Digits, bin%mod)
}

// SecondsLeft until the code at t rolls over.
func (g *Generator) SecondsLeft(t time.Time) int {
	return g.Period - int(t.Unix()%int64(g.Period))
}
