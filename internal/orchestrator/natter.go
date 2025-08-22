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
	"natter/internal/upnp"
)

// Natter is the core orchestrator: sets up port mapping, forwarding, keep-alive, and status updates.
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
	bindIP   net.IP
}

// New creates a Natter instance with configuration and logger.
func New(cfg *config.Config, logger *zap.Logger) (*Natter, error) {
	// Initialize STUN client
	stunCli := stun.NewClient(cfg.StunServer.TCP, cfg.StunServer.UDP, time.Second, logger)
	// Initialize status manager
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

	// Parse open ports
	for _, a := range cfg.OpenPort.TCP {
		h, p := splitAddr(a)
		n.tcpOpens = append(n.tcpOpens, net.TCPAddr{IP: net.ParseIP(h), Port: p})
	}
	for _, a := range cfg.OpenPort.UDP {
		h, p := splitAddr(a)
		n.udpOpens = append(n.udpOpens, net.UDPAddr{IP: net.ParseIP(h), Port: p})
	}

	// Prepare forwarders
	if len(cfg.OpenPort.TCP) == len(cfg.ForwardPort.TCP) {
		// 一一对应模式
		for i, target := range cfg.ForwardPort.TCP {
			listenAddr := cfg.OpenPort.TCP[i] // e.g. "0.0.0.0:33887"
			fwd := forward.NewTCPForwarder(listenAddr, target, logger)
			n.tcpFwds = append(n.tcpFwds, fwd)
		}
	} else {
		// 旧逻辑：监听目标端口
		for _, target := range cfg.ForwardPort.TCP {
			listenAddr := "0.0.0.0:" + portOf(target)
			fwd := forward.NewTCPForwarder(listenAddr, target, logger)
			n.tcpFwds = append(n.tcpFwds, fwd)
		}
	}

	return n, nil
}

func portOf(addr string) string {
	idx := strings.LastIndex(addr, ":")
	return addr[idx+1:]
}

// Run starts UPnP mapping, status manager, forwarders, keep-alive, and STUN workers until context cancel.
func (n *Natter) Run(ctx context.Context) {
	if n.bindIP == nil || n.bindIP.IsUnspecified() {
		n.bindIP = n.getOutboundIP() // 建议换成固定 DNS，如 "119.29.29.29:53" 的探路实现
	}
	n.logger.Info("bind ip decided", zap.String("bind_ip", n.bindIP.String()))
	n.stunClient.SetBindIP(n.bindIP)

	// UPnP port mapping if enabled
	if n.cfg.EnableUPnP {
		cli, err := upnp.Discover(n.logger)
		if err != nil {
			n.logger.Warn("UPnP discovery failed", zap.Error(err))
		} else {
			for _, addr := range n.tcpOpens {
				// Determine actual inner IP (replace 0.0.0.0)
				innerIP := addr.IP.String()
				if addr.IP.IsUnspecified() {
					innerIP = n.getOutboundIP().String()
				}
				// Add UPnP mapping: external and internal ports are the same
				if err := cli.AddTCP(addr.Port, addr.Port, innerIP, 0); err != nil {
					n.logger.Warn("UPnP AddTCP failed", zap.Int("port", addr.Port), zap.Error(err))
				} else {
					n.logger.Info("UPnP TCP map added", zap.String("inner", fmt.Sprintf("%s:%d", innerIP, addr.Port)), zap.Int("port", addr.Port))
				}
			}
			for _, addr := range n.udpOpens {
				// Determine actual inner IP (replace 0.0.0.0)
				innerIP := addr.IP.String()
				if addr.IP.IsUnspecified() {
					innerIP = n.getOutboundIP().String()
				}
				// Add UPnP mapping for UDP
				if err := cli.AddUDP(addr.Port, addr.Port, innerIP, 0); err != nil {
					n.logger.Warn("UPnP AddUDP failed", zap.Int("port", addr.Port), zap.Error(err))
				} else {
					n.logger.Info("UPnP UDP map added", zap.String("inner", fmt.Sprintf("%s:%d", innerIP, addr.Port)), zap.Int("port", addr.Port))
				}
			}
		}
	}

	// Start status manager
	go n.statusMgr.Run(ctx)

	// Start forwarders
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

	// Open port tasks: keep-alive + mapping detection
	for _, a := range n.tcpOpens {
		addr := a // ✅ 复制一份，避免 &addr 指向同一个循环变量
		// keepalive 绑定到“真实本地 IP:监听端口”
		laddr := &net.TCPAddr{IP: n.bindIP, Port: addr.Port}
		go keepalive.TCPKeepAlive(ctx, laddr, n.cfg.KeepAlive, n.interval, n.logger)
		go n.runWorker(ctx, "tcp", &addr)
	}
	for _, a := range n.udpOpens {
		// Listen for UDP Keep-Alive
		addr := a
		pc, err := net.ListenPacket("udp", addr.String())
		if err != nil {
			n.logger.Warn("UDP listen failed", zap.Error(err))
		} else {
			go keepalive.UDPKeepAlive(ctx, pc, n.cfg.KeepAlive, addr.Port, n.interval, n.logger)
		}
		// Run STUN worker
		go n.runWorker(ctx, "udp", &addr)
	}

	// Block until context done
	<-ctx.Done()
	n.logger.Info("Natter shutting down")
}

// runWorker polls STUN for mapping and pushes updates.
func (n *Natter) runWorker(ctx context.Context, proto string, addr net.Addr) {
	inner := formatInner(addr, n.getOutboundIP())
	lastOuter := ""
	for {
		var outer string
		var err error
		if proto == "tcp" {
			res, e := n.stunClient.GetTCPMapping(addr.(*net.TCPAddr).Port)
			err = e
			if err == nil {
				outer = fmt.Sprintf("%s:%d", res.ExternalIP, res.ExternalPort)
			}
		} else {
			res, e := n.stunClient.GetUDPMapping(addr.(*net.UDPAddr).Port)
			err = e
			if err == nil {
				outer = fmt.Sprintf("%s:%d", res.ExternalIP, res.ExternalPort)
			}
		}
		if err != nil {
			n.logger.Debug("STUN mapping failed", zap.String("proto", proto), zap.Error(err))
		} else if outer != lastOuter {
			n.statusMgr.Updates <- status.UpdateEvent{Protocol: proto, InnerAddr: inner, OuterAddr: outer}
			lastOuter = outer
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(n.interval):
		}
	}
}

// getOutboundIP returns the machine's preferred outbound IP.
func (n *Natter) getOutboundIP() net.IP {
	// 用 IPv4 目的地址探路，强制走 IPv4 路径
	c, err := net.Dial("udp4", "119.29.29.29:53") // 或 "1.1.1.1:53"
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP.To4()
	if ip == nil {
		return net.IPv4(127, 0, 0, 1)
	}
	return ip
}

// formatInner formats the inner address, replacing 0.0.0.0 with actual IP.
func formatInner(addr net.Addr, outboundIP net.IP) string {
	s := addr.String()
	if strings.HasPrefix(s, "0.0.0.0:") {
		parts := strings.Split(s, ":")
		return fmt.Sprintf("%s:%s", outboundIP.String(), parts[1])
	}
	return s
}

// splitAddr splits "host:port" into host and port int.
func splitAddr(a string) (string, int) {
	p := strings.LastIndex(a, ":")
	h := a[:p]
	pi, _ := strconv.Atoi(a[p+1:])
	return h, pi
}
