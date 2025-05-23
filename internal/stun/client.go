package stun

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun"
	"go.uber.org/zap"
)

// Mapping 表示 STUN 映射的内部/外部地址
type Mapping struct {
	InternalIP   net.IP
	InternalPort int
	ExternalIP   net.IP
	ExternalPort int
}

// Client 是 STUN 客户端，用于获取 UDP/TCP 映射
type Client struct {
	tcpServers []string
	udpServers []string
	timeout    time.Duration
	logger     *zap.Logger
}

// NewClient 创建一个 STUN 客户端实例。
// tcpServers, udpServers 是 STUN 服务器域名或 IP 列表；timeout 用于连接和请求的超时时间；logger 用于日志。
func NewClient(tcpServers, udpServers []string, timeout time.Duration, logger *zap.Logger) *Client {
	return &Client{
		tcpServers: tcpServers,
		udpServers: udpServers,
		timeout:    timeout,
		logger:     logger,
	}
}

// GetUDPMapping 获取给定本地 UDP 端口的映射地址
func (c *Client) GetUDPMapping(srcPort int) (*Mapping, error) {
	for _, server := range c.udpServers {
		addr := fmt.Sprintf("%s:3478", server)
		c.logger.Debug("STUN UDP dialing", zap.String("server", addr))

		// 本地监听指定端口
		laddr := &net.UDPAddr{IP: net.IPv4zero, Port: srcPort}
		raddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			c.logger.Warn("Failed to resolve STUN server", zap.String("server", server), zap.Error(err))
			continue
		}

		conn, err := net.DialUDP("udp", laddr, raddr)
		if err != nil {
			c.logger.Warn("UDP dial failed", zap.String("server", server), zap.Error(err))
			continue
		}
		conn.SetDeadline(time.Now().Add(c.timeout))

		// 构建绑定请求
		message := stun.MustBuild(stun.BindingRequest, stun.TransactionID, stun.Fingerprint)

		// 创建 STUN 事务客户端
		client, _ := stun.NewClient(conn)
		defer client.Close()

		var xorAddr stun.XORMappedAddress
		err = client.Do(message, func(ev stun.Event) {
			if ev.Error != nil {
				err = ev.Error
				return
			}
			if getErr := xorAddr.GetFrom(ev.Message); getErr != nil {
				err = getErr
			}
		})
		conn.Close()
		if err != nil {
			c.logger.Warn("STUN transaction failed", zap.String("server", server), zap.Error(err))
			continue
		}

		mapping := &Mapping{
			InternalIP:   laddr.IP,
			InternalPort: laddr.Port,
			ExternalIP:   xorAddr.IP,
			ExternalPort: xorAddr.Port,
		}
		return mapping, nil
	}
	return nil, fmt.Errorf("all UDP STUN servers failed")
}

// GetTCPMapping 获取给定本地 TCP 端口的映射地址。
// 注意：不同服务器支持情况略有差异。
func (c *Client) GetTCPMapping(srcPort int) (*Mapping, error) {
	for _, server := range c.tcpServers {
		addr := fmt.Sprintf("%s:3478", server)
		c.logger.Debug("STUN TCP dialing", zap.String("server", addr))

		// 建立 TCP 连接并绑定本地端口
		laddr := &net.TCPAddr{IP: net.IPv4zero, Port: srcPort}
		raddr, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			c.logger.Warn("Failed to resolve STUN server", zap.String("server", server), zap.Error(err))
			continue
		}
		conn, err := net.DialTCP("tcp", laddr, raddr)
		if err != nil {
			c.logger.Warn("TCP dial failed", zap.String("server", server), zap.Error(err))
			continue
		}
		conn.SetDeadline(time.Now().Add(c.timeout))

		// 构建绑定请求
		message := stun.MustBuild(stun.BindingRequest, stun.TransactionID, stun.Fingerprint)

		// 创建 STUN 事务客户端
		client, _ := stun.NewClient(conn)
		defer client.Close()

		var xorAddr stun.XORMappedAddress
		err = client.Do(message, func(ev stun.Event) {
			if ev.Error != nil {
				err = ev.Error
				return
			}
			if getErr := xorAddr.GetFrom(ev.Message); getErr != nil {
				err = getErr
			}
		})
		conn.Close()
		if err != nil {
			c.logger.Warn("STUN TCP transaction failed", zap.String("server", server), zap.Error(err))
			continue
		}

		mapping := &Mapping{
			InternalIP:   laddr.IP,
			InternalPort: laddr.Port,
			ExternalIP:   xorAddr.IP,
			ExternalPort: xorAddr.Port,
		}
		return mapping, nil
	}
	return nil, fmt.Errorf("all TCP STUN servers failed")
}
