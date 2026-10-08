// Package helper is the privileged side of Tunnelkey: it runs openvpn for the
// app and reports its state. It never stores credentials; they arrive with a
// connect request and are handed to openvpn over its management interface.
package helper

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ovpn"
)

const (
	// ConnectTimeout gives up on servers that can't be reached, like the phone app.
	ConnectTimeout = 30 * time.Second
	// credsTimeout is how long a reconnect waits for the app to supply credentials.
	credsTimeout = 90 * time.Second
	stopTimeout  = 8 * time.Second
	logLimit     = 1000
)

// Version is set by the build.
var Version = "dev"

// Engine owns at most one openvpn session.
type Engine struct {
	mu     sync.Mutex
	status ipc.Status
	logs   []string
	subs   map[*ipc.Conn]bool
	sess   *session
	nextID int
}

type session struct {
	id         int
	cmd        *exec.Cmd
	dir        string
	m          *mgmt
	creds      *ipc.Credentials
	usedCreds  bool
	stopping   bool
	connected  bool
	failure    ipc.Failure
	message    string
	credsCh    chan ipc.Credentials
	done       chan struct{}
	staticChal bool
	// awaitingCreds: waiting for the app, so the connect timeout doesn't apply.
	awaitingCreds bool
}

// New returns an idle engine.
func New() *Engine {
	return &Engine{status: ipc.Status{Phase: ipc.Disconnected}, subs: map[*ipc.Conn]bool{}}
}

// Serve accepts app connections until the listener is closed.
func (e *Engine) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go e.handle(ipc.Wrap(c))
	}
}

func (e *Engine) handle(c *ipc.Conn) {
	defer func() {
		e.mu.Lock()
		delete(e.subs, c)
		e.mu.Unlock()
		c.Close()
	}()
	for {
		var req ipc.Request
		if err := c.Read(&req); err != nil {
			return
		}
		switch req.Op {
		case "hello":
			e.mu.Lock()
			e.subs[c] = true
			st := e.status
			logs := append([]string(nil), e.logs...)
			e.mu.Unlock()
			c.Write(ipc.Event{Type: "hello", Version: ipc.ProtocolVersion, Message: Version})
			c.Write(ipc.Event{Type: "status", Status: &st})
			if len(logs) > 0 {
				c.Write(ipc.Event{Type: "log", Lines: logs})
			}
		case "connect":
			go e.Connect(req)
		case "credentials":
			e.provide(req.Credentials)
		case "disconnect":
			go e.Disconnect()
		default:
			c.Write(ipc.Event{Type: "error", Message: "unknown request " + req.Op})
		}
	}
}

// ---- State --------------------------------------------------------------

func (e *Engine) update(f func(*ipc.Status)) {
	e.mu.Lock()
	f(&e.status)
	st := e.status
	subs := e.subscribers()
	e.mu.Unlock()
	for _, c := range subs {
		c.Write(ipc.Event{Type: "status", Status: &st})
	}
}

func (e *Engine) log(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	line = time.Now().Format("15:04:05 ") + line
	e.mu.Lock()
	e.logs = append(e.logs, line)
	if len(e.logs) > logLimit {
		e.logs = e.logs[len(e.logs)-logLimit:]
	}
	subs := e.subscribers()
	e.mu.Unlock()
	for _, c := range subs {
		c.Write(ipc.Event{Type: "log", Lines: []string{line}})
	}
}

func (e *Engine) broadcast(ev ipc.Event) {
	e.mu.Lock()
	subs := e.subscribers()
	e.mu.Unlock()
	for _, c := range subs {
		c.Write(ev)
	}
}

func (e *Engine) subscribers() []*ipc.Conn {
	out := make([]*ipc.Conn, 0, len(e.subs))
	for c := range e.subs {
		out = append(out, c)
	}
	return out
}

func (e *Engine) current(s *session) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sess == s
}

// ---- Connect / disconnect ----------------------------------------------

// Connect replaces any running session with a new one.
func (e *Engine) Connect(req ipc.Request) {
	e.stopAndWait()

	e.mu.Lock()
	e.nextID++
	s := &session{id: e.nextID, creds: req.Credentials, credsCh: make(chan ipc.Credentials, 1), done: make(chan struct{})}
	e.sess = s
	e.logs = nil
	e.mu.Unlock()

	e.update(func(st *ipc.Status) {
		*st = ipc.Status{Phase: ipc.Connecting, ProfileID: req.ProfileID, ProfileName: req.ProfileName,
			Server: ovpn.Inspect(req.Config).Remote}
	})
	e.log("Connecting to %s", req.ProfileName)

	if err := e.start(s, req); err != nil {
		e.log("%v", err)
		failure := ipc.Other
		var pe *ovpn.Error
		switch {
		case errors.As(err, &pe):
			failure = ipc.BadProfile
		case errors.Is(err, errNoOpenVPN):
			failure = ipc.NoEngine
		}
		e.fail(s, failure, err.Error(), false)
		e.finish(s)
	}
}

// Disconnect stops the running session, if any.
func (e *Engine) Disconnect() {
	e.mu.Lock()
	s := e.sess
	e.mu.Unlock()
	if s == nil || s.cmd == nil {
		e.update(func(st *ipc.Status) {
			if st.Phase != ipc.Failed {
				st.Phase = ipc.Disconnected
			}
		})
		return
	}
	e.stop(s)
}

func (e *Engine) stop(s *session) {
	e.mu.Lock()
	already := s.stopping
	s.stopping = true
	e.mu.Unlock()
	if already {
		return
	}
	e.update(func(st *ipc.Status) { st.Phase = ipc.Disconnecting })
	e.mu.Lock()
	m := s.m
	e.mu.Unlock()
	if m != nil {
		m.send("signal SIGTERM")
	} else if s.cmd != nil && s.cmd.Process != nil {
		terminate(s.cmd.Process)
	}
	go func() {
		select {
		case <-s.done:
		case <-time.After(stopTimeout):
			e.log("openvpn didn't stop within %s; ending it", stopTimeout)
			if s.cmd != nil && s.cmd.Process != nil {
				s.cmd.Process.Kill()
			}
		}
	}()
}

func (e *Engine) stopAndWait() {
	e.mu.Lock()
	s := e.sess
	e.mu.Unlock()
	if s == nil || s.cmd == nil {
		return
	}
	e.stop(s)
	select {
	case <-s.done:
	case <-time.After(stopTimeout + 2*time.Second):
	}
}

func (e *Engine) provide(c *ipc.Credentials) {
	if c == nil {
		return
	}
	e.mu.Lock()
	s := e.sess
	e.mu.Unlock()
	if s == nil {
		return
	}
	select {
	case s.credsCh <- *c:
	default:
	}
}

// ---- Session ------------------------------------------------------------

var errNoOpenVPN = errors.New("OpenVPN is not installed")

// Replaced in tests.
var (
	findOpenVPN    = openvpnPath
	makeSessionDir = sessionDir
	extraArgs      = platformArgs
)

func (e *Engine) start(s *session, req ipc.Request) error {
	config, err := ovpn.Sanitize(req.Config)
	if err != nil {
		return err
	}
	s.staticChal = ovpn.Inspect(config).HasStaticChallenge

	exe, err := findOpenVPN()
	if err != nil {
		return errNoOpenVPN
	}
	dir, err := makeSessionDir(s.id)
	if err != nil {
		return fmt.Errorf("could not prepare a private directory: %w", err)
	}
	s.dir = dir

	cfgPath := filepath.Join(dir, "profile.ovpn")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		return err
	}
	secret := randomHex(24)
	pwPath := filepath.Join(dir, "management.pw")
	if err := os.WriteFile(pwPath, []byte(secret+"\n"), 0o600); err != nil {
		return err
	}
	mgmtArgs, dial, err := management(dir, pwPath)
	if err != nil {
		return err
	}

	args := []string{
		"--config", cfgPath,
		"--management-query-passwords", "--management-hold",
		"--auth-retry", "none", "--verb", "3",
		"--setenv", "IV_GUI_VER", "Tunnelkey_desktop_" + Version,
	}
	args = append(args, mgmtArgs...)
	args = append(args, extraArgs(dir)...)

	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	prepareCommand(cmd)
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start OpenVPN: %w", err)
	}
	s.cmd = cmd
	go e.pipeOutput(out)
	go func() {
		err := cmd.Wait()
		e.mu.Lock()
		stopping := s.stopping
		e.mu.Unlock()
		if err != nil && !stopping {
			e.log("OpenVPN exited: %v", err)
		}
		e.finish(s)
	}()

	conn, err := dialWithRetry(dial, 10*time.Second, s.done)
	if err != nil {
		e.stop(s)
		return fmt.Errorf("could not reach OpenVPN's management interface: %w", err)
	}
	m := &mgmt{conn: conn, r: bufio.NewReader(conn)}
	if err := m.login(secret); err != nil {
		conn.Close()
		e.stop(s)
		return err
	}
	e.mu.Lock()
	s.m = m
	e.mu.Unlock()
	go e.readManagement(s)
	for _, c := range []string{"state on", "bytecount 2", "log on", "hold release"} {
		s.m.send(c)
	}
	go e.watchConnectTimeout(s)
	return nil
}

func (m *mgmt) login(secret string) error {
	m.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer m.conn.SetReadDeadline(time.Time{})
	// openvpn prompts "ENTER PASSWORD:" without a newline.
	buf := make([]byte, 0, 64)
	one := make([]byte, 1)
	for !strings.HasSuffix(string(buf), "ENTER PASSWORD:") {
		if _, err := m.r.Read(one); err != nil {
			return fmt.Errorf("management login: %w", err)
		}
		buf = append(buf, one[0])
		if len(buf) > 4096 {
			return errors.New("management login: unexpected greeting")
		}
	}
	if err := m.send(secret); err != nil {
		return err
	}
	for {
		line, err := m.readLine()
		if err != nil {
			return fmt.Errorf("management login: %w", err)
		}
		if strings.HasPrefix(line, "SUCCESS:") {
			return nil
		}
		if strings.HasPrefix(line, "ERROR:") {
			return errors.New("management login refused")
		}
	}
}

func dialWithRetry(dial func() (net.Conn, error), limit time.Duration, done <-chan struct{}) (net.Conn, error) {
	deadline := time.Now().Add(limit)
	for {
		c, err := dial()
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-done:
			return nil, errors.New("OpenVPN exited during start-up")
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (e *Engine) pipeOutput(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		// Before the management interface is up, errors only appear here.
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.Contains(line, "MANAGEMENT:") {
			e.log("%s", line)
		}
	}
}

func (e *Engine) watchConnectTimeout(s *session) {
	select {
	case <-s.done:
	case <-time.After(ConnectTimeout):
		e.mu.Lock()
		stale := s.connected || s.stopping || e.sess != s || s.awaitingCreds
		e.mu.Unlock()
		if !stale {
			e.log("No connection after %s; giving up", ConnectTimeout)
			e.fail(s, ipc.Timeout, fmt.Sprintf("The server didn't answer within %d seconds.", int(ConnectTimeout.Seconds())), false)
			e.stop(s)
		}
	}
}

func (e *Engine) readManagement(s *session) {
	for {
		line, err := s.m.readLine()
		if err != nil {
			return
		}
		if !strings.HasPrefix(line, ">") {
			continue // command replies
		}
		kind, payload, _ := strings.Cut(line[1:], ":")
		switch kind {
		case "STATE":
			e.onState(s, parseState(payload))
		case "BYTECOUNT":
			if in, out, ok := parseByteCount(payload); ok && e.current(s) {
				e.update(func(st *ipc.Status) { st.BytesIn, st.BytesOut = in, out })
			}
		case "LOG":
			e.log("%s", parseLog(payload))
		case "PASSWORD":
			e.onPassword(s, parsePassword(payload))
		case "FATAL":
			e.log("Fatal: %s", payload)
			e.fail(s, ipc.Other, payload, true)
		case "HOLD":
			s.m.send("hold release")
		}
	}
}

func (e *Engine) onState(s *session, st stateLine) {
	if !e.current(s) {
		return
	}
	switch st.Name {
	case "CONNECTED":
		e.mu.Lock()
		s.connected = true
		e.mu.Unlock()
		e.update(func(x *ipc.Status) {
			x.Phase, x.Step, x.Failure, x.Message = ipc.Connected, st.Name, "", ""
			x.VPNAddress = st.LocalIP
			if st.RemoteIP != "" {
				x.Server = st.RemoteIP + ":" + st.RemotePort
			}
			if x.ConnectedAt == 0 {
				x.ConnectedAt = time.Now().UnixMilli()
			}
		})
	case "RECONNECTING":
		if strings.Contains(st.Detail, "auth") {
			e.fail(s, ipc.AuthFailed, "", false)
		}
		e.update(func(x *ipc.Status) {
			if !s.stopping {
				x.Phase = ipc.Reconnecting
			}
			x.Step = st.Name
		})
	case "EXITING":
		if strings.Contains(st.Detail, "auth") {
			e.fail(s, ipc.AuthFailed, "", true)
		}
	default:
		e.update(func(x *ipc.Status) { x.Step = st.Name })
	}
}

func (e *Engine) onPassword(s *session, p passwordPrompt) {
	switch {
	case p.Failed:
		msg := ""
		if p.DynamicChallenge {
			msg = "The server asked for an additional challenge, which Tunnelkey doesn't support."
		}
		e.fail(s, ipc.AuthFailed, msg, false)
		e.log("Sign-in was rejected")
	case p.Need && p.Type == "Auth":
		go e.answerCredentials(s, p.StaticChallenge || s.staticChal)
	case p.Need:
		// Private key passphrases etc. are not supported.
		e.log("OpenVPN asked for %q, which Tunnelkey can't provide", p.Type)
		e.fail(s, ipc.BadProfile, "The profile needs a "+p.Type+" password, which Tunnelkey doesn't support.", false)
		e.stop(s)
	}
}

func (e *Engine) answerCredentials(s *session, staticChallenge bool) {
	var c ipc.Credentials
	e.mu.Lock()
	first := s.creds != nil && !s.usedCreds
	if first {
		c = *s.creds
		s.usedCreds = true
		s.creds = nil
	}
	e.mu.Unlock()
	if !first {
		// A reconnect needs a fresh sign-in (the code has changed).
		e.mu.Lock()
		s.awaitingCreds = true
		e.mu.Unlock()
		e.broadcast(ipc.Event{Type: "need-creds"})
		defer func() {
			e.mu.Lock()
			s.awaitingCreds = false
			e.mu.Unlock()
		}()
		select {
		case c = <-s.credsCh:
		case <-s.done:
			return
		case <-time.After(credsTimeout):
			e.fail(s, ipc.NeedsSignIn, "", false)
			e.stop(s)
			return
		}
	}
	password := c.Password
	if staticChallenge {
		password = staticChallengeResponse(c.Password, c.Response)
	}
	for _, cmd := range credentialCommands("Auth", c.Username, password) {
		s.m.send(cmd)
	}
}

// finish runs once when the openvpn process has gone (or never started).
func (e *Engine) finish(s *session) {
	e.mu.Lock()
	select {
	case <-s.done:
		e.mu.Unlock()
		return
	default:
		close(s.done)
	}
	latest := e.sess == s
	if latest {
		e.sess = nil
	}
	failure, message, stopping := s.failure, s.message, s.stopping
	e.mu.Unlock()

	e.mu.Lock()
	m := s.m
	e.mu.Unlock()
	if m != nil {
		m.conn.Close()
	}
	if s.dir != "" {
		os.RemoveAll(s.dir)
	}
	if !latest {
		return
	}
	e.update(func(st *ipc.Status) {
		if stopping && failure != ipc.Timeout && failure != ipc.NeedsSignIn && failure != ipc.BadProfile {
			failure = ""
		}
		if failure != "" {
			st.Phase, st.Failure, st.Message = ipc.Failed, failure, message
		} else {
			st.Phase, st.Failure, st.Message = ipc.Disconnected, "", ""
		}
		st.ConnectedAt, st.VPNAddress, st.Step = 0, "", ""
	})
	e.log("Session ended")
}

// fail records why a session ends; keepFirst keeps an earlier reason.
func (e *Engine) fail(s *session, f ipc.Failure, msg string, keepFirst bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if keepFirst && s.failure != "" {
		return
	}
	s.failure = f
	if msg != "" || !keepFirst {
		s.message = msg
	}
}

// Shutdown stops the session; used when the service stops.
func (e *Engine) Shutdown() { e.stopAndWait() }

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
