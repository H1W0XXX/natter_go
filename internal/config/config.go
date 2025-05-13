package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// StunServer 配置 STUN 服务器列表
type StunServer struct {
	TCP []string `json:"tcp"`
	UDP []string `json:"udp"`
}

// OpenPort 配置待检测的开放端口
type OpenPort struct {
	TCP []string `json:"tcp"` // 形式: "IP:Port"
	UDP []string `json:"udp"`
}

// ForwardPort 配置需要转发的目标地址
type ForwardPort struct {
	TCP []string `json:"tcp"`
	UDP []string `json:"udp"`
}

// StatusReport 配置状态报告文件及 Hook
type StatusReport struct {
	Hook       string `json:"hook"`
	StatusFile string `json:"status_file"`
}

// Logging 配置日志等级和文件
type Logging struct {
	Level   string `json:"level"`    // "debug", "info", etc.
	LogFile string `json:"log_file"` // 可选路径，"" 表示不写文件
}

// Config 是整个配置文件结构
// Interval 单位为秒，用于控制映射检测和保活间隔
type Config struct {
	StunServer   StunServer   `json:"stun_server"`
	KeepAlive    string       `json:"keep_alive"`
	Interval     int          `json:"interval"`
	OpenPort     OpenPort     `json:"open_port"`
	ForwardPort  ForwardPort  `json:"forward_port"`
	StatusReport StatusReport `json:"status_report"`
	Logging      Logging      `json:"logging"`
}

// Load 从 JSON 配置文件加载 Config
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}
