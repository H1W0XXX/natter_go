//go:build linux || darwin

package stun

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func newBoundDialer(laddr *net.TCPAddr, timeout time.Duration) net.Dialer {
	return net.Dialer{
		LocalAddr: laddr,
		Timeout:   timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			return err
		},
	}
}
