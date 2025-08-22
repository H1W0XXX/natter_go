//go:build linux || darwin

package forward

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func listenWithReuse(ctx context.Context, addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			return err
		},
	}
	return lc.Listen(ctx, "tcp4", addr)
}
