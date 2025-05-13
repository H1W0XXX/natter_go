package orchestrator

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"natter/internal/config"
	"natter/internal/forward"
	"natter/internal/keepalive"
	"natter/internal/status"
	"natter/internal/stun"
)

// Natter 核心编排器
// 负责：启动转发器、保活、映射检测、状态管理
type Natter struct {
	cfg        *config.Config
	logger     *zap.Logger
	stunClient *stun.Client
	statusMgr  *status.StatusManager
	interval   time.Duration

	tcpOpens []net.TCPAddr
	udpOpens []net.UDPAddr
	tcpFwds  []*forward.TCPForwarder
	udpFwds  []*forward.UDPForwarder
}

// New 创建 Natter 实例
func New(cfg *config.Config, logger *zap.Logger) (*Natter, error) {
	// STUN 客户端
	stunCli := stun.NewClient(cfg.StunServer.TCP, cfg.StunServer.UDP, time.Second*1, logger)
	// 状态管理
	sm, err := status.NewManager(cfg.StatusReport.StatusFile, cfg.StatusReport.Hook, logger)
	if err != nil {
		return nil, err
	}

	n := &Natter{
		cfg:        cfg,
		logger:     logger,
		stunClient: stunCli,
		statusMgr:  sm,
		interval:   time.Duration(cfg.Interval) * time.Second,
	}

	// 解析 open_port
	for _, addrStr := range cfg.OpenPort.TCP {
		parts := strings.Split(addrStr, ":")
		host, port := parts[0], parts[1]
		pi, _ := strconv.Atoi(port)
		n.tcpOpens = append(n.tcpOpens, net.TCPAddr{IP: net.ParseIP(host), Port: pi})
	}
	for _, addrStr := range cfg.OpenPort.UDP {
		parts := strings.Split(addrStr, ":")
		host, port := parts[0], parts[1]
		pi, _ := strconv.Atoi(port)
		n.udpOpens = append(n.udpOpens, net.UDPAddr{IP: net.ParseIP(host), Port: pi})
	}

	// 设置 forward_port 转发器
	for _, addrStr := range cfg.ForwardPort.TCP {
		// 本地随机端口 "0.0.0.0:0"
		n.tcpFwds = append(n.tcpFwds, forward.NewTCPForwarder("0.0.0.0:0", addrStr, logger))
	}
	for _, addrStr := range cfg.ForwardPort.UDP {
		// 本地随机端口 "0.0.0.0:0"
		n.udpFwds = append(n.udpFwds, forward.NewUDPForwarder("0.0.0.0:0", addrStr, n.interval, logger))
	}

	return n, nil
}

// Run 启动所有子服务，并阻塞直到 ctx 结束
func (n *Natter) Run(ctx context.Context) {
	// 状态管理
	go n.statusMgr.Run(ctx)

	// 启动转发器
	for _, fw := range n.tcpFwds {
		if err := fw.Start(ctx); err != nil {
			n.logger.Warn("TCP forwarder start failed", zap.Error(err))
		}
	}
	for _, fw := range n.udpFwds {
		if err := fw.Start(ctx); err != nil {
			n.logger.Warn("UDP forwarder start failed", zap.Error(err))
		}
	}

	// 启动 OpenPort 任务
	for _, addr := range n.tcpOpens {
		// 映射检测
		go n.runTCPWorker(ctx, addr)
		// TCP 保活
		go keepalive.TCPKeepAlive(ctx, addr.IP.String(), n.cfg.KeepAlive, n.interval, n.logger)
	}
	for _, addr := range n.udpOpens {
		// UDP 保活: 复用监听 port 的 PacketConn
		pc, err := net.ListenPacket("udp", addr.String())
		if err != nil {
			n.logger.Warn("UDP listen failed for keepalive", zap.String("addr", addr.String()), zap.Error(err))
		} else {
			go keepalive.UDPKeepAlive(ctx, pc, n.cfg.KeepAlive, addr.Port, n.interval, n.logger)
		}
		// 映射检测
		go n.runUDPWorker(ctx, addr)
	}

	// 阻塞到退出信号
	<-ctx.Done()
	n.logger.Info("Natter shutting down")
}

// runTCPWorker 周期性获取 TCP 映射并发送更新事件
func (n *Natter) runTCPWorker(ctx context.Context, addr net.TCPAddr) {
	inner := addr.String()
	var lastOuter string
	for {
		mapRes, err := n.stunClient.GetTCPMapping(addr.Port)
		if err == nil {
			outer := fmt.Sprintf("%s:%d", mapRes.ExternalIP, mapRes.ExternalPort)
			if outer != lastOuter {
				n.statusMgr.Updates <- status.UpdateEvent{Protocol: "tcp", InnerAddr: inner, OuterAddr: outer}
				lastOuter = outer
			}
		} else {
			n.logger.Debug("TCP mapping failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(n.interval):
		}
	}
}

// runUDPWorker 周期性获取 UDP 映射并发送更新事件
func (n *Natter) runUDPWorker(ctx context.Context, addr net.UDPAddr) {
	inner := addr.String()
	var lastOuter string
	for {
		mapRes, err := n.stunClient.GetUDPMapping(addr.Port)
		if err == nil {
			outer := fmt.Sprintf("%s:%d", mapRes.ExternalIP, mapRes.ExternalPort)
			if outer != lastOuter {
				n.statusMgr.Updates <- status.UpdateEvent{Protocol: "udp", InnerAddr: inner, OuterAddr: outer}
				lastOuter = outer
			}
		} else {
			n.logger.Debug("UDP mapping failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(n.interval):
		}
	}
}
