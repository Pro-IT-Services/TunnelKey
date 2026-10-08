package helper

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// openvpnPath finds openvpn.exe from the official OpenVPN installation that
// the Tunnelkey installer sets up.
func openvpnPath() (string, error) {
	for _, view := range []uint32{registry.WOW64_64KEY, 0} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\OpenVPN`, registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		exe, _, err := k.GetStringValue("exe_path")
		k.Close()
		if err == nil && exe != "" {
			if _, err := os.Stat(exe); err == nil {
				return exe, nil
			}
		}
	}
	exe := filepath.Join(os.Getenv("ProgramFiles"), "OpenVPN", "bin", "openvpn.exe")
	if _, err := os.Stat(exe); err == nil {
		return exe, nil
	}
	return "", errNoOpenVPN
}

// Only SYSTEM and Administrators may read the session directory: it holds
// the management password.
const runDirSDDL = "D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)"

func sessionDir(id int) (string, error) {
	base := filepath.Join(os.Getenv("ProgramData"), "Tunnelkey", "run")
	if err := mkdirPrivate(filepath.Dir(base)); err != nil {
		return "", err
	}
	if err := mkdirPrivate(base); err != nil {
		return "", err
	}
	dir := filepath.Join(base, "s"+strconv.Itoa(id)+"-"+randomHex(4))
	return dir, mkdirPrivate(dir)
}

func mkdirPrivate(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(runDirSDDL)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafeSizeofSA), SecurityDescriptor: sd}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	err = windows.CreateDirectory(p, sa)
	if err == windows.ERROR_ALREADY_EXISTS {
		// Reapply the DACL in case someone pre-created the directory.
		dacl, _, derr := sd.DACL()
		if derr != nil {
			return derr
		}
		return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	}
	return err
}

// management uses TCP on the loopback with a password file (openvpn on
// Windows has no Unix sockets).
func management(dir, pwPath string) ([]string, func() (net.Conn, error), error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return []string{"--management", "127.0.0.1", strconv.Itoa(port), pwPath},
		func() (net.Conn, error) { return net.Dial("tcp", addr) }, nil
}

// OpenVPN on Windows sets routes and DNS itself.
func platformArgs(string) []string { return nil }

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// Windows has no SIGTERM; the management interface is the clean way.
func terminate(p *os.Process) { p.Kill() }
