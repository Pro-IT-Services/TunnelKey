package ipc

import (
	"context"
	"testing"
)

func TestPipeNameAndListen(t *testing.T) {
	if PipeName != `\\.\pipe\tunnelkey-helper` {
		t.Fatalf("pipe name %q", PipeName)
	}
	if c, err := Dial(context.Background()); err == nil {
		c.Close()
		t.Skip("the installed Tunnelkey helper already owns the pipe")
	}
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
}
