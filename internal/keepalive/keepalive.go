package keepalive

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

// TCPKeepAlive 定期向指定主机的 80 端口发送 HTTP HEAD 请求，保持 NAT 连接
func TCPKeepAlive(ctx context.Context, sourceHost string, host string, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("TCPKeepAlive exiting")
			return
		case <-ticker.C:
			addr := fmt.Sprintf("%s:80", host)
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				logger.Debug("TCP keepalive dial failed", zap.String("host", host), zap.Error(err))
				continue
			}

			// 发送最少量的 HEAD 请求
			head := fmt.Sprintf("HEAD / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
			conn.Write([]byte(head))
			conn.Close()
			logger.Debug("TCP keepalive sent", zap.String("host", host))
		}
	}
}

// UDPKeepAlive 定期向指定主机的 UDP 端口发送空包，保持 NAT 连接
func UDPKeepAlive(ctx context.Context, conn net.PacketConn, host string, port int, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	raddr := &net.UDPAddr{IP: net.ParseIP(host), Port: port}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("UDPKeepAlive exiting")
			return
		case <-ticker.C:
			// 发送空包或简单消息
			if _, err := conn.WriteTo([]byte("hello"), raddr); err != nil {
				logger.Debug("UDP keepalive failed", zap.Error(err))
			} else {
				logger.Debug("UDP keepalive sent", zap.String("to", raddr.String()))
			}
		}
	}
}
