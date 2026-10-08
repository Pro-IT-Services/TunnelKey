package helper

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kalipsers/TunnelKey/desktop/internal/ipc"
)

// The test binary doubles as a fake openvpn that speaks the management
// protocol, so the engine can be tested without OpenVPN installed.
func TestMain(m *testing.M) {
	if os.Getenv("TUNNELKEY_FAKE_OPENVPN") == "1" {
		fakeOpenVPN(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeOpenVPN(args []string) {
	var addr, network, pwFile string
	for i, a := range args {
		if a == "--management" {
			if args[i+2] == "unix" {
				network, addr, pwFile = "unix", args[i+1], args[i+3]
			} else {
				network, addr, pwFile = "tcp", args[i+1]+":"+args[i+2], args[i+3]
			}
		}
	}
	want, _ := os.ReadFile(pwFile)
	l, err := net.Listen(network, addr)
	if err != nil {
		os.Exit(2)
	}
	c, _ := l.Accept()
	r := bufio.NewReader(c)
	fmt.Fprint(c, "ENTER PASSWORD:")
	if pw, _ := r.ReadString('\n'); pw != string(want) {
		os.Exit(3)
	}
	fmt.Fprint(c, "SUCCESS: password is correct\r\n>HOLD:Waiting for hold release:0\r\n")
	var user string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			os.Exit(4)
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "hold release":
			fmt.Fprint(c, ">STATE:1,WAIT,,,,,,\r\n>PASSWORD:Need 'Auth' username/password\r\n")
		case strings.HasPrefix(line, "username "):
			user = line
		case strings.HasPrefix(line, "password "):
			if user == `username "Auth" "marko"` && line == `password "Auth" "pw123456"` {
				fmt.Fprint(c, ">STATE:2,CONNECTED,SUCCESS,10.8.0.2,1.2.3.4,1194,,\r\n>BYTECOUNT:100,200\r\n")
			} else {
				fmt.Fprint(c, ">PASSWORD:Verification Failed: 'Auth'\r\n>STATE:3,EXITING,auth-failure,,,,,\r\n")
				os.Exit(0)
			}
		case line == "signal SIGTERM":
			fmt.Fprint(c, ">STATE:4,EXITING,SIGTERM,,,,,\r\n")
			os.Exit(0)
		}
	}
}

func setupFake(t *testing.T) {
	exe, _ := os.Executable()
	t.Setenv("TUNNELKEY_FAKE_OPENVPN", "1")
	base := t.TempDir()
	findOpenVPN = func() (string, error) { return exe, nil }
	makeSessionDir = func(id int) (string, error) {
		d := filepath.Join(base, fmt.Sprint(id))
		return d, os.Mkdir(d, 0o700)
	}
	extraArgs = func(string) []string { return nil }
}

const profile = "client\ndev tun\nremote vpn.example.com 1194 udp\nauth-user-pass\n<ca>\nX\n</ca>\n"

func waitPhase(t *testing.T, e *Engine, want ipc.Phase) ipc.Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		st := e.status
		e.mu.Unlock()
		if st.Phase == want {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	t.Fatalf("phase %s not reached; status %+v; log:\n%s", want, e.status, strings.Join(e.logs, "\n"))
	return ipc.Status{}
}

func TestConnectAndDisconnect(t *testing.T) {
	setupFake(t)
	e := New()
	go e.Connect(ipc.Request{ProfileName: "Office", Config: profile,
		Credentials: &ipc.Credentials{Username: "marko", Password: "pw123456"}})
	st := waitPhase(t, e, ipc.Connected)
	if st.VPNAddress != "10.8.0.2" || st.Server != "1.2.3.4:1194" || st.ConnectedAt == 0 {
		t.Fatalf("unexpected status %+v", st)
	}
	e.Disconnect()
	waitPhase(t, e, ipc.Disconnected)
}

func TestAuthFailure(t *testing.T) {
	setupFake(t)
	e := New()
	go e.Connect(ipc.Request{ProfileName: "Office", Config: profile,
		Credentials: &ipc.Credentials{Username: "marko", Password: "wrong"}})
	st := waitPhase(t, e, ipc.Failed)
	if st.Failure != ipc.AuthFailed {
		t.Fatalf("failure %q", st.Failure)
	}
}

func TestRefusesUnsafeProfile(t *testing.T) {
	setupFake(t)
	e := New()
	e.Connect(ipc.Request{Config: profile + "plugin /tmp/x.so\n"})
	st := waitPhase(t, e, ipc.Failed)
	if st.Failure != ipc.BadProfile {
		t.Fatalf("failure %q", st.Failure)
	}
}

func TestParsers(t *testing.T) {
	if p := parsePassword("Need 'Auth' username/password SC:1,Enter code"); !p.Need || !p.StaticChallenge || p.Type != "Auth" {
		t.Fatalf("%+v", p)
	}
	if got := staticChallengeResponse("pw", "123456"); got != "SCRV1:cHc=:MTIzNDU2" {
		t.Fatal(got)
	}
	if got := quote(`a"b\c`); got != "\"a\\\"b\\\\c\"" {
		t.Fatal(got)
	}
	c := pushedDNS(func(k string) string {
		return map[string]string{
			"foreign_option_1": "dhcp-option DNS 10.0.0.1", "foreign_option_2": "dhcp-option DOMAIN corp.example",
			"route_network_1": "0.0.0.0",
		}[k]
	})
	if len(c.Servers) != 1 || c.Domains[0] != "corp.example" || !c.FullTunnel {
		t.Fatalf("%+v", c)
	}
}
