package helper

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
)

// mgmt talks to openvpn's management interface.
type mgmt struct {
	conn net.Conn
	r    *bufio.Reader
	wmu  sync.Mutex
}

func (m *mgmt) send(cmd string) error {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	_, err := m.conn.Write([]byte(cmd + "\n"))
	return err
}

// readLine returns the next line without the trailing CR/LF.
func (m *mgmt) readLine() (string, error) {
	line, err := m.r.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

// quote escapes a value for a management command argument.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// staticChallengeResponse builds the SCRV1 password openvpn expects when a
// profile uses static-challenge.
func staticChallengeResponse(password, response string) string {
	enc := base64.StdEncoding.EncodeToString
	return "SCRV1:" + enc([]byte(password)) + ":" + enc([]byte(response))
}

// stateLine is a parsed ">STATE:" notification.
type stateLine struct {
	Name, Detail, LocalIP, RemoteIP, RemotePort string
}

func parseState(payload string) stateLine {
	// time,name,detail,local_ip,remote_ip,remote_port,local_addr,local_port,local_ipv6
	f := strings.Split(payload, ",")
	get := func(i int) string {
		if i < len(f) {
			return f[i]
		}
		return ""
	}
	return stateLine{Name: get(1), Detail: get(2), LocalIP: get(3), RemoteIP: get(4), RemotePort: get(5)}
}

func parseByteCount(payload string) (in, out int64, ok bool) {
	a, b, found := strings.Cut(payload, ",")
	if !found {
		return 0, 0, false
	}
	i, err1 := strconv.ParseInt(a, 10, 64)
	o, err2 := strconv.ParseInt(b, 10, 64)
	return i, o, err1 == nil && err2 == nil
}

// parseLog turns ">LOG:time,flags,text" into the text.
func parseLog(payload string) string {
	parts := strings.SplitN(payload, ",", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return payload
}

// passwordPrompt describes a ">PASSWORD:" notification.
type passwordPrompt struct {
	Need             bool   // credentials requested
	Failed           bool   // "Verification Failed"
	StaticChallenge  bool   // SC: present
	DynamicChallenge bool   // CRV1 (not supported)
	Type             string // usually "Auth"
}

func parsePassword(payload string) passwordPrompt {
	var p passwordPrompt
	switch {
	case strings.HasPrefix(payload, "Verification Failed"):
		p.Failed = true
		if strings.Contains(payload, "CRV1") {
			p.DynamicChallenge = true
		}
	case strings.HasPrefix(payload, "Need '"):
		rest := strings.TrimPrefix(payload, "Need '")
		typ, after, _ := strings.Cut(rest, "'")
		p.Type = typ
		p.Need = strings.Contains(after, "username/password") || strings.Contains(after, "password")
		p.StaticChallenge = strings.Contains(after, "SC:")
	}
	return p
}

func credentialCommands(typ, username, password string) []string {
	return []string{
		fmt.Sprintf("username %s %s", quote(typ), quote(username)),
		fmt.Sprintf("password %s %s", quote(typ), quote(password)),
	}
}
