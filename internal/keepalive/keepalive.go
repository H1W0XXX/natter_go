package keepalive

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	mr "math/rand"
	"net"
	"time"

	"go.uber.org/zap"
)

// minInterval 保底 5 秒
func minInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return 5 * time.Second
	}
	return d
}

// TCPKeepAlive 与 Python v2.1 版一致的改进：
// 1. 持久连接保持 5 元组；失败后指数退避重连
// 2. 支持 host 为域名，先在 DialContext 时解析
// 3. 绑定本地 laddr
func TCPKeepAlive(ctx context.Context, laddr *net.TCPAddr, host string, interval time.Duration, logger *zap.Logger) {
	interval = minInterval(interval)
	hostPort := net.JoinHostPort(host, "80")

	var conn *net.TCPConn
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	backoff := interval

	for {
		if conn == nil {
			dialer := net.Dialer{LocalAddr: laddr, Timeout: 3 * time.Second}
			c, err := dialer.DialContext(ctx, "tcp", hostPort)
			if err != nil {
				logger.Debug("TCP keepalive dial failed", zap.String("host", host), zap.Error(err))
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff = time.Duration(math.Min(float64(backoff*2), float64(60*time.Second)))
				continue
			}
			conn = c.(*net.TCPConn)
			_ = conn.SetNoDelay(true)
			logger.Debug("TCP keepalive connection established", zap.String("local", conn.LocalAddr().String()))
			backoff = interval
		}

		req := fmt.Sprintf("HEAD /natter-keep-alive HTTP/1.1\r\nHost: %s\r\nConnection: keep-alive\r\n\r\n", host)
		if _, err := io.WriteString(conn, req); err != nil {
			logger.Debug("TCP keepalive write failed", zap.Error(err))
			conn.Close()
			conn = nil
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		buf := make([]byte, 4)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					break
				}
				logger.Debug("TCP keepalive read failed", zap.Error(err))
				conn.Close()
				conn = nil
				break
			}
		}
		if conn != nil {
			logger.Debug("TCP keepalive ok", zap.String("remote", hostPort))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// UDPKeepAlive 发送 DNS 查询帧
// UDPKeepAlive 发送 DNS 查询帧；支持 host 为域名
func UDPKeepAlive(ctx context.Context, conn net.PacketConn, host string, port int, interval time.Duration, logger *zap.Logger) {
	interval = minInterval(interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 解析 host → IP（每次都解析，兼容动态解析）
	resolve := func() *net.UDPAddr {
		if ip := net.ParseIP(host); ip != nil {
			return &net.UDPAddr{IP: ip, Port: port}
		}
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprint(port)))
		if err != nil {
			logger.Debug("UDP keepalive resolve failed", zap.Error(err))
			return nil
		}
		return addr
	}

	for {
		raddr := resolve()
		if raddr == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		txid := make([]byte, 2)
		if _, err := rand.Read(txid); err != nil {
			binary.BigEndian.PutUint16(txid, uint16(mr.Intn(0xffff)))
		}
		header := append(txid, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
		qname := []byte{0x09, 'k', 'e', 'e', 'p', 'a', 'l', 'i', 'v', 'e', 0x06, 'n', 'a', 't', 't', 'e', 'r', 0x00}
		question := []byte{0x00, 0x01, 0x00, 0x01}
		pkt := append(header, append(qname, question...)...)

		if _, err := conn.WriteTo(pkt, raddr); err != nil {
			logger.Debug("UDP keepalive failed", zap.Error(err))
		} else {
			logger.Debug("UDP keepalive sent", zap.String("to", raddr.String()))
		}

		select {
		case <-ctx.Done():
			logger.Debug("UDPKeepAlive exiting")
			return
		case <-ticker.C:
		}
	}
}
