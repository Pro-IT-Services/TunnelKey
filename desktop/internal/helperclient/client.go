// Package helperclient keeps the app connected to the privileged helper and
// reconnects when the helper restarts.
package helperclient

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kalipsers/TunnelKey/desktop/internal/ipc"
)

// ErrNotRunning means the helper service can't be reached.
var ErrNotRunning = errors.New("the Tunnelkey helper service is not running")

// Client delivers helper events to OnEvent and OnConnection.
type Client struct {
	OnEvent      func(ipc.Event)
	OnConnection func(connected bool, helperVersion string)

	mu   sync.Mutex
	conn *ipc.Conn
}

// Run connects until ctx ends.
func (c *Client) Run(ctx context.Context) {
	for ctx.Err() == nil {
		raw, err := ipc.Dial(ctx)
		if err != nil {
			c.OnConnection(false, "")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		conn := ipc.Wrap(raw)
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()
		conn.Write(ipc.Request{Op: "hello", Version: ipc.ProtocolVersion})
		go func() {
			<-ctx.Done()
			conn.Close()
		}()
		for {
			var ev ipc.Event
			if err := conn.Read(&ev); err != nil {
				break
			}
			if ev.Type == "hello" {
				c.OnConnection(true, ev.Message)
				continue
			}
			c.OnEvent(ev)
		}
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		conn.Close()
		c.OnConnection(false, "")
	}
}

// Send a request to the helper.
func (c *Client) Send(req ipc.Request) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrNotRunning
	}
	return conn.Write(req)
}
