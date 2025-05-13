package forward

import (
	"context"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// UDPForwarder 将本地 ListenAddr 上的 UDP 包转发到 TargetAddr。
// 为每个客户端地址维护一个到服务器的 UDP 连接，并反向转发响应。
type UDPForwarder struct {
	ListenAddr string
	TargetAddr string
	Timeout    time.Duration
	logger     *zap.Logger

	conn      *net.UDPConn
	clients   map[string]*net.UDPConn
	clientsMu sync.Mutex
	wg        sync.WaitGroup
}

// NewUDPForwarder 创建一个 UDP 转发器。
// listenAddr, targetAddr: 格式 "host:port"；timeout：可选读写超时时间；logger：用于日志输出。
func NewUDPForwarder(listenAddr, targetAddr string, timeout time.Duration, logger *zap.Logger) *UDPForwarder {
	return &UDPForwarder{
		ListenAddr: listenAddr,
		TargetAddr: targetAddr,
		Timeout:    timeout,
		logger:     logger,
		clients:    make(map[string]*net.UDPConn),
	}
}

// Start 启动 UDP 转发器，监听本地端口并开始处理。
func (f *UDPForwarder) Start(ctx context.Context) error {
	laddr, err := net.ResolveUDPAddr("udp", f.ListenAddr)
	if err != nil {
		f.logger.Error("resolve listen address failed", zap.String("addr", f.ListenAddr), zap.Error(err))
		return err
	}
	f.conn, err = net.ListenUDP("udp", laddr)
	if err != nil {
		f.logger.Error("listen UDP failed", zap.String("addr", f.ListenAddr), zap.Error(err))
		return err
	}
	f.logger.Info("UDP forwarder listening", zap.String("listen", f.ListenAddr), zap.String("target", f.TargetAddr))

	f.wg.Add(1)
	go f.acceptLoop(ctx)
	return nil
}

// acceptLoop 接收客户端数据并转发到目标服务器。
func (f *UDPForwarder) acceptLoop(ctx context.Context) {
	defer f.wg.Done()
	buf := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, clientAddr, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			f.logger.Debug("UDP read error", zap.Error(err))
			continue
		}

		key := clientAddr.String()

		// 获取或创建客户端->服务器的连接
		f.clientsMu.Lock()
		srvConn, ok := f.clients[key]
		if !ok {
			// 建立到 TargetAddr 的 UDP 连接
			raddr, err := net.ResolveUDPAddr("udp", f.TargetAddr)
			if err != nil {
				f.logger.Warn("resolve target address failed", zap.String("target", f.TargetAddr), zap.Error(err))
				f.clientsMu.Unlock()
				continue
			}

			srvConn, err = net.DialUDP("udp", nil, raddr)
			if err != nil {
				f.logger.Warn("dial target UDP failed", zap.String("target", f.TargetAddr), zap.Error(err))
				f.clientsMu.Unlock()
				continue
			}

			// 启动反向转发协程
			f.wg.Add(1)
			go f.handleServerResponse(clientAddr, srvConn)

			f.clients[key] = srvConn
		}
		f.clientsMu.Unlock()

		// 写数据到目标服务器
		if _, err := srvConn.Write(buf[:n]); err != nil {
			f.logger.Debug("write to server failed", zap.Error(err))
		}
	}
}

// handleServerResponse 读取服务器响应并转发回客户端。
func (f *UDPForwarder) handleServerResponse(clientAddr *net.UDPAddr, srvConn *net.UDPConn) {
	defer f.wg.Done()
	buf := make([]byte, 2048)

	for {
		srvConn.SetReadDeadline(time.Now().Add(f.Timeout))
		n, err := srvConn.Read(buf)
		if err != nil {
			// 超时或连接关闭后清理
			f.logger.Debug("server UDP read closed", zap.Error(err))
			break
		}

		// 将数据写回客户端
		if _, err := f.conn.WriteToUDP(buf[:n], clientAddr); err != nil {
			f.logger.Debug("write back to client failed", zap.Error(err))
		}
	}

	// 清理
	key := clientAddr.String()
	f.clientsMu.Lock()
	srvConn.Close()
	delete(f.clients, key)
	f.clientsMu.Unlock()
}

// Stop 优雅关闭 UDP 转发器，等待所有 goroutine 退出。
func (f *UDPForwarder) Stop() {
	if f.conn != nil {
		f.conn.Close()
	}
	// 关闭所有客户端连接
	f.clientsMu.Lock()
	for _, c := range f.clients {
		c.Close()
	}
	f.clientsMu.Unlock()

	f.wg.Wait()
	f.logger.Info("UDP forwarder stopped", zap.String("listen", f.ListenAddr))
}
