// Package ipc is the protocol between the Tunnelkey app and the privileged
// helper that runs openvpn: newline-delimited JSON over a named pipe
// (Windows) or a Unix socket (macOS, Linux).
package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
)

// ProtocolVersion is bumped on incompatible changes.
const ProtocolVersion = 1

// Phase of the tunnel, mirrors the phone apps.
type Phase string

const (
	Disconnected  Phase = "disconnected"
	Connecting    Phase = "connecting"
	Connected     Phase = "connected"
	Reconnecting  Phase = "reconnecting"
	Disconnecting Phase = "disconnecting"
	Failed        Phase = "failed"
)

// Failure explains a Failed phase.
type Failure string

const (
	AuthFailed  Failure = "auth_failed"
	NeedsSignIn Failure = "needs_sign_in"
	BadProfile  Failure = "profile"
	Timeout     Failure = "timeout"
	NoEngine    Failure = "no_engine"
	Other       Failure = "other"
)

// Status is the helper's view of the tunnel.
type Status struct {
	Phase       Phase   `json:"phase"`
	Failure     Failure `json:"failure,omitempty"`
	Message     string  `json:"message,omitempty"`
	Step        string  `json:"step,omitempty"`
	ProfileID   string  `json:"profileId,omitempty"`
	ProfileName string  `json:"profileName,omitempty"`
	Server      string  `json:"server,omitempty"`
	VPNAddress  string  `json:"vpnAddress,omitempty"`
	ConnectedAt int64   `json:"connectedAt,omitempty"` // unix ms
	BytesIn     int64   `json:"bytesIn"`
	BytesOut    int64   `json:"bytesOut"`
}

// Credentials for auth-user-pass. Response is the static-challenge answer.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Response string `json:"response,omitempty"`
}

// Request from the app.
type Request struct {
	Op          string       `json:"op"` // hello, connect, credentials, disconnect
	Version     int          `json:"version,omitempty"`
	ProfileID   string       `json:"profileId,omitempty"`
	ProfileName string       `json:"profileName,omitempty"`
	Config      string       `json:"config,omitempty"`
	Credentials *Credentials `json:"credentials,omitempty"`
}

// Event from the helper.
type Event struct {
	Type    string   `json:"type"` // status, log, need-creds, error
	Status  *Status  `json:"status,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Message string   `json:"message,omitempty"`
	Version int      `json:"version,omitempty"`
}

// Conn wraps a stream with JSON framing; writes are serialised.
type Conn struct {
	c   net.Conn
	r   *bufio.Reader
	wmu sync.Mutex
}

// Wrap a connected stream.
func Wrap(c net.Conn) *Conn {
	return &Conn{c: c, r: bufio.NewReaderSize(c, 64*1024)}
}

// maxLine bounds one message (a profile is at most 256 KB).
const maxLine = 1 << 20

// Read decodes the next message into v.
func (c *Conn) Read(v any) error {
	var line []byte
	for {
		chunk, isPrefix, err := c.r.ReadLine()
		if err != nil {
			return err
		}
		line = append(line, chunk...)
		if len(line) > maxLine {
			return bufio.ErrTooLong
		}
		if !isPrefix {
			break
		}
	}
	return json.Unmarshal(line, v)
}

// Write encodes v as one line.
func (c *Conn) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.c.Write(append(b, '\n'))
	return err
}

// Close the stream.
func (c *Conn) Close() error { return c.c.Close() }
