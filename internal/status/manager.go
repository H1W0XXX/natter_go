package status

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// UpdateEvent 表示一个映射更新事件
type UpdateEvent struct {
	Protocol  string // "tcp" 或 "udp"
	InnerAddr string // 格式 "IP:Port"
	OuterAddr string // 格式 "IP:Port"
}

// StatusManager 管理 NAT 映射状态，写入文件并执行 Hook
type StatusManager struct {
	Updates chan UpdateEvent
	hookCmd string
	file    *os.File
	logger  *zap.Logger

	mutex    sync.Mutex
	mappings map[string]map[string]string // protocol -> inner -> outer
}

// NewManager 创建一个 StatusManager
// filePath: 状态文件路径，hookCmd: 可选的命令模板，支持 {inner} {outer} 占位符
func NewManager(filePath, hookCmd string, logger *zap.Logger) (*StatusManager, error) {
	// 打开或创建文件
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("open status file: %w", err)
	}

	m := &StatusManager{
		Updates:  make(chan UpdateEvent, 100),
		hookCmd:  hookCmd,
		file:     f,
		logger:   logger,
		mappings: map[string]map[string]string{"tcp": {}, "udp": {}},
	}
	return m, nil
}

// Run 启动状态管理循环，直到 ctx 结束
func (m *StatusManager) Run(ctx context.Context) {
	m.logger.Info("StatusManager started")
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("StatusManager exiting")
			m.file.Close()
			return

		case ev := <-m.Updates:
			m.handleEvent(ev)
		}
	}
}

// handleEvent 处理单次更新
func (m *StatusManager) handleEvent(ev UpdateEvent) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	protocolMap := m.mappings[ev.Protocol]
	old, exists := protocolMap[ev.InnerAddr]
	if exists && old == ev.OuterAddr {
		// 未变化，跳过
		return
	}
	// 更新映射
	protocolMap[ev.InnerAddr] = ev.OuterAddr
	m.logger.Info("Mapping updated", zap.String("protocol", ev.Protocol), zap.String("inner", ev.InnerAddr), zap.String("outer", ev.OuterAddr))

	// 写入文件
	if err := m.writeFile(); err != nil {
		m.logger.Warn("Failed to write status file", zap.Error(err))
	}

	// 执行 Hook
	if m.hookCmd != "" {
		cmdStr := m.expandHook(ev)
		m.logger.Debug("Executing hook", zap.String("cmd", cmdStr))
		exec.CommandContext(context.Background(), "sh", "-c", cmdStr).Start()
	}
}

// writeFile 将当前 mappings 写入 JSON 文件
func (m *StatusManager) writeFile() error {
	// 准备结构
	tmp := map[string][]map[string]string{"tcp": {}, "udp": {}}
	for protocol, amap := range m.mappings {
		for inner, outer := range amap {
			rec := map[string]string{"inner": inner, "outer": outer}
			tmp[protocol] = append(tmp[protocol], rec)
		}
	}

	// 清空并写入
	if _, err := m.file.Seek(0, 0); err != nil {
		return err
	}
	if err := m.file.Truncate(0); err != nil {
		return err
	}

	enc := json.NewEncoder(m.file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tmp); err != nil {
		return err
	}
	return nil
}

// expandHook 用实际地址替换占位符
func (m *StatusManager) expandHook(ev UpdateEvent) string {
	s := m.hookCmd
	s = strings.ReplaceAll(s, "{inner}", ev.InnerAddr)
	s = strings.ReplaceAll(s, "{outer}", ev.OuterAddr)
	s = strings.ReplaceAll(s, "{protocol}", ev.Protocol)
	return s
}
