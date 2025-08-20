//go:build windows

package stun

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const soExclusiveAddrUse = 0x0004

// 绑定到指定本地 IP:Port，关闭排他占用，开启 REUSEADDR
func newBoundDialer(laddr *net.TCPAddr, timeout time.Duration) net.Dialer {
	return net.Dialer{
		LocalAddr: laddr,
		Timeout:   timeout,
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
