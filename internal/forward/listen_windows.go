// internal/forward/listen_windows.go
//go:build windows

package forward

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

const soExclusiveAddrUse = 0x0004 // WinSock 常量

func listenWithReuse(ctx context.Context, addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				// 关闭排他占用，允许另一个 socket(主动连接)绑定同端口
				_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, soExclusiveAddrUse, 0)
				// 开启 REUSEADDR（Windows 没有通用的 REUSEPORT 语义）
				_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
			})
			return err
		},
	}
	return lc.Listen(ctx, "tcp4", addr)
}
