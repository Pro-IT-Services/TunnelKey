package ipc

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// PipeName of the helper.
const PipeName = `\\.\pipe\tunnelkey-helper`

// SYSTEM and Administrators: full access; interactive users: read/write.
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)"

// Dial connects to the helper.
func Dial(ctx context.Context) (net.Conn, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return winio.DialPipeContext(c, PipeName)
}

// Listen opens the helper's endpoint.
func Listen() (net.Listener, error) {
	return winio.ListenPipe(PipeName, &winio.PipeConfig{
		SecurityDescriptor: pipeSDDL,
		InputBufferSize:    64 * 1024,
		OutputBufferSize:   64 * 1024,
	})
}
