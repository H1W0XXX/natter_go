package forward

import (
	"context"
	"io"
	"net"
	"sync"

	"go.uber.org/zap"
)

// TCPForwarder 将本地 ListenAddr 上的 TCP 连接转发到 TargetAddr。
type TCPForwarder struct {
	ListenAddr string
	TargetAddr string
	logger     *zap.Logger

	listener net.Listener
	wg       sync.WaitGroup
}

// NewTCPForwarder 创建一个 TCP 转发器。
func NewTCPForwarder(listenAddr, targetAddr string, logger *zap.Logger) *TCPForwarder {
	return &TCPForwarder{
		ListenAddr: listenAddr,
		TargetAddr: targetAddr,
		logger:     logger,
	}
}

// Start 启动转发器，开始监听并接受连接。
// ctx 用于优雅关闭。
func (f *TCPForwarder) Start(ctx context.Context) error {
	ln, err := listenWithReuse(ctx, f.ListenAddr)
	if err != nil {
		f.logger.Error("cannot listen on TCP address", zap.String("addr", f.ListenAddr), zap.Error(err))
		return err
	}
	f.listener = ln
	f.logger.Info("TCP forwarder listening", zap.String("listen", f.ListenAddr), zap.String("target", f.TargetAddr))

	f.wg.Add(1)
	go f.acceptLoop(ctx)
	return nil
}

// acceptLoop 接受客户端连接并派发处理。
func (f *TCPForwarder) acceptLoop(ctx context.Context) {
	defer f.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		clientConn, err := f.listener.Accept()
		if err != nil {
			// 关闭时会返回错误，退出循环
			f.logger.Debug("TCP accept error", zap.Error(err))
			return
		}
		f.logger.Debug("Accepted TCP client", zap.String("client", clientConn.RemoteAddr().String()))

		f.wg.Add(1)
		go func(src net.Conn) {
			defer f.wg.Done()
			f.handleConnection(src)
		}(clientConn)
	}
}

// handleConnection 建立到目标的连接并开始双向转发。
func (f *TCPForwarder) handleConnection(src net.Conn) {
	defer src.Close()
	// 链接目标
	dst, err := net.Dial("tcp", f.TargetAddr)
	if err != nil {
		f.logger.Warn("TCP dial to target failed", zap.String("target", f.TargetAddr), zap.Error(err))
		return
	}
	defer dst.Close()

	// 双向拷贝
	f.logger.Debug("Forwarding TCP data", zap.String("from", src.RemoteAddr().String()), zap.String("to", f.TargetAddr))
	var p sync.WaitGroup
	p.Add(2)
	go func() {
		io.Copy(dst, src)
		p.Done()
	}()
	go func() {
		io.Copy(src, dst)
		p.Done()
	}()
	p.Wait()
}

// Stop 优雅关闭转发器，等待所有连接处理完成。
func (f *TCPForwarder) Stop() {
	if f.listener != nil {
		f.listener.Close()
	}
	f.wg.Wait()
	f.logger.Info("TCP forwarder stopped", zap.String("listen", f.ListenAddr))
}
