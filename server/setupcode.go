package main

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Setup code encoding, see docs/provisioning-format.md.

const (
	singleCodeMax = 2900 // Base45 chars that still go into one QR (ECC L)
	partMax       = 1500 // Base45 chars per part when splitting (ECC M)
)

type payloadTOTP struct {
	Secret    string `json:"s"`
	Digits    int    `json:"d"`
	Period    int    `json:"p"`
	Algorithm string `json:"a"`
}

type payloadLink struct {
	Title string `json:"t"`
	Kind  string `json:"k"`
	URI   string `json:"u"`
}

type payload struct {
	Version      int           `json:"v"`
	Name         string        `json:"n"`
	OVPN         string        `json:"o"`
	Username     string        `json:"u,omitempty"`
	Password     string        `json:"p,omitempty"`
	TOTP         *payloadTOTP  `json:"t,omitempty"`
	ManualCode   bool          `json:"f,omitempty"`
	CodePosition string        `json:"c,omitempty"`
	Links        []payloadLink `json:"l,omitempty"`
}

func buildPayload(p *Package) payload {
	out := payload{
		Version:      1,
		Name:         p.Name,
		OVPN:         normalizeNewlines(p.OVPN),
		Username:     p.Username,
		Password:     p.Password,
		ManualCode:   p.TOTP == nil && p.ManualCode,
		CodePosition: "a",
	}
	if p.CodePosition == "before" {
		out.CodePosition = "b"
	}
	if p.TOTP != nil {
		t := *p.TOTP
		out.TOTP = &payloadTOTP{Secret: t.Secret, Digits: t.Digits, Period: t.Period, Algorithm: t.Algorithm}
	}
	if out.TOTP == nil && !out.ManualCode {
		out.CodePosition = ""
	}
	for _, l := range p.Links {
		out.Links = append(out.Links, payloadLink{Title: l.Title, Kind: l.Kind, URI: l.URI()})
	}
	return out
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimPrefix(s, "\uFEFF")
}

// URI returns what the phone opens for this link.
func (l Link) URI() string {
	if l.Kind != "rdp" {
		return l.URL
	}
	port := l.Port
	if port == 0 {
		port = 3389
	}
	// Microsoft Remote Desktop URI scheme: name=type:value pairs, spaces as %20.
	var b strings.Builder
	b.WriteString("rdp://full%20address=s:")
	b.WriteString(rdpEscape(l.Host + ":" + strconv.Itoa(port)))
	if l.Username != "" {
		b.WriteString("&username=s:")
		b.WriteString(rdpEscape(l.Username))
	}
	return b.String()
}

// rdpEscape percent-encodes only what would break the URI; ':' stays literal
// as in Microsoft's documented examples (full%20address=s:host:3389).
var rdpEscaper = strings.NewReplacer("%", "%25", " ", "%20", "&", "%26", "=", "%3D", "\\", "%5C", "#", "%23", "?", "%3F")

func rdpEscape(s string) string {
	return rdpEscaper.Replace(s)
}

// encodeSetupCodes returns the text of each QR code in the set.
func encodeSetupCodes(p payload) ([]string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var z bytes.Buffer
	w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	w.Write(raw)
	w.Close()
	text := base45Encode(z.Bytes())

	n := 1
	if len(text) > singleCodeMax {
		n = (len(text) + partMax - 1) / partMax
	}
	id, err := randomID(6)
	if err != nil {
		return nil, err
	}
	size := (len(text) + n - 1) / n
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		start, end := i*size, min((i+1)*size, len(text))
		part := text[start:end]
		codes = append(codes, fmt.Sprintf("TK1:%d/%d:%s:%d:%s", i+1, n, id, len(part), part))
	}
	return codes, nil
}

// decodeSetupCodes is the reference reader (used by tests).
func decodeSetupCodes(codes []string) (payload, error) {
	var out payload
	parts := map[int]string{}
	total, id := 0, ""
	for _, c := range codes {
		f := strings.SplitN(c, ":", 5)
		if len(f) != 5 || f[0] != "TK1" {
			return out, errors.New("not a setup code")
		}
		var i, n int
		if _, err := fmt.Sscanf(f[1], "%d/%d", &i, &n); err != nil || i < 1 || i > n {
			return out, errors.New("bad part number")
		}
		if id == "" {
			id, total = f[2], n
		} else if f[2] != id || n != total {
			continue
		}
		l, err := strconv.Atoi(f[3])
		if err != nil {
			return out, err
		}
		data := f[4]
		for len(data) < l {
			data += " "
		}
		parts[i] = data
	}
	if len(parts) != total {
		return out, fmt.Errorf("have %d of %d parts", len(parts), total)
	}
	var b strings.Builder
	for i := 1; i <= total; i++ {
		b.WriteString(parts[i])
	}
	compressed, err := base45Decode(b.String())
	if err != nil {
		return out, err
	}
	r, err := zlib.NewReader(bytes.NewReader(compressed))
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

const base45Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

func base45Encode(in []byte) string {
	var b strings.Builder
	for i := 0; i+1 < len(in); i += 2 {
		v := int(in[i])*256 + int(in[i+1])
		b.WriteByte(base45Alphabet[v%45])
		b.WriteByte(base45Alphabet[(v/45)%45])
		b.WriteByte(base45Alphabet[v/2025])
	}
	if len(in)%2 == 1 {
		v := int(in[len(in)-1])
		b.WriteByte(base45Alphabet[v%45])
		b.WriteByte(base45Alphabet[v/45])
	}
	return b.String()
}

func base45Decode(s string) ([]byte, error) {
	vals := make([]int, len(s))
	for i := 0; i < len(s); i++ {
		v := strings.IndexByte(base45Alphabet, s[i])
		if v < 0 {
			return nil, fmt.Errorf("invalid base45 character %q", s[i])
		}
		vals[i] = v
	}
	if len(vals)%3 == 1 {
		return nil, errors.New("invalid base45 length")
	}
	out := make([]byte, 0, len(vals)*2/3)
	for i := 0; i < len(vals); i += 3 {
		if i+2 < len(vals) {
			v := vals[i] + vals[i+1]*45 + vals[i+2]*2025
			if v > 0xFFFF {
				return nil, errors.New("invalid base45 triplet")
			}
			out = append(out, byte(v>>8), byte(v))
		} else {
			v := vals[i] + vals[i+1]*45
			if v > 0xFF {
				return nil, errors.New("invalid base45 pair")
			}
			out = append(out, byte(v))
		}
	}
	return out, nil
}

func randomID(n int) (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf), nil
}
