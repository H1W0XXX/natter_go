//go:build linux || darwin

package keepalive

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func newDialerWithReuse(laddr *net.TCPAddr) net.Dialer {
	return net.Dialer{
		LocalAddr: laddr,
		Timeout:   3 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				// Linux/部分 *nix 支持 SO_REUSEPORT
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			return err
		},
	}
}
