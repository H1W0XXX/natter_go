// Package upnp discovers the local IGD (Internet Gateway Device) and adds
// port‑mapping rules so that a machine behind NAT can expose a TCP/UDP port.
//
// Notes for Windows:
//   - Windows 自带的家庭/路由器设备几乎都支持 IGDv1/v2；
//   - 代码基于 github.com/huin/goupnp/v2（最成熟的 UPnP 实现），
//     直接 `go get github.com/huin/goupnp/v2@latest` 即可；
//   - 只在程序启动时尝试一次，如果失败不会影响主逻辑。
//
// Example:
//
//	cli, _ := upnp.Discover(logger)
//	_ = cli.AddTCP(33888, 33888, "192.168.1.199")
//	// 外网 33888 → 192.168.1.199:33888
package upnp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway1"
	"go.uber.org/zap"
)

// Client wraps a WANIPConnection1 service.
// Only minimal methods required by Natter are exposed.
// If Discover returns (nil, err) caller should treat UPnP as unavailable.
//
// The methods are safe to call concurrently.
//
// Zero‑value is not valid – must come from Discover().
type Client struct {
	svc    *internetgateway1.WANIPConnection1
	logger *zap.Logger
}

// Discover searches for the first IGD that exposes WANIPConnection1.
// Typical latency < 1s。若找不到设备，返回 (nil, error)。
func Discover(logger *zap.Logger) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	devs, err, _ := internetgateway1.NewWANIPConnection1ClientsCtx(ctx)
	if err != nil {
		return nil, fmt.Errorf("upnp discover: %w", err)
	}
	if len(devs) == 0 {
		return nil, fmt.Errorf("upnp discover: no IGD found")
	}
	cli := &Client{svc: devs[0], logger: logger}
	logger.Info("UPnP IGD found", zap.String("url", devs[0].Location.String()))
	return cli, nil
}

// AddTCP maps externalPort on the gateway to internalIP:internalPort (TCP).
// durationSec = 0 代表永久映射。
func (c *Client) AddTCP(externalPort, internalPort int, internalIP string, durationSec uint32) error {
	return c.add("TCP", externalPort, internalPort, internalIP, durationSec)
}

// AddUDP maps UDP port.
func (c *Client) AddUDP(externalPort, internalPort int, internalIP string, durationSec uint32) error {
	return c.add("UDP", externalPort, internalPort, internalIP, durationSec)
}

func (c *Client) add(proto string, ext, in int, host string, dur uint32) error {
	if net.ParseIP(host) == nil {
		return fmt.Errorf("invalid internal IP: %s", host)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// remoteHost="" 表示映射所有来源
	if err := c.svc.AddPortMappingCtx(ctx, "", uint16(ext), proto, uint16(in), host, true, "natter-go", dur); err != nil {
		return fmt.Errorf("add port‑mapping (%s %d): %w", proto, ext, err)
	}
	c.logger.Info("UPnP port‑mapping added", zap.String("proto", proto), zap.Int("outer", ext), zap.String("inner", fmt.Sprintf("%s:%d", host, in)))
	return nil
}
