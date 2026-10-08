package ipc

import "testing"

func TestPipeNameAndListen(t *testing.T) {
	if PipeName != `\\.\pipe\tunnelkey-helper` {
		t.Fatalf("pipe name %q", PipeName)
	}
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
}
