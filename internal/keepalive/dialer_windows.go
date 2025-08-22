//go:build windows

package keepalive

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const soExclusiveAddrUse = 0x0004

func newDialerWithReuse(laddr *net.TCPAddr) net.Dialer {
	return net.Dialer{
		LocalAddr: laddr,
		Timeout:   3 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, soExclusiveAddrUse, 0)
				_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
			})
			return err
		},
	}
}
