package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 创建并返回一个 zap.Logger，根据传入的 levelStr 和可选的 logFilePath。
// levelStr 支持 "debug", "info", "warn", "error" 等级别。
// logFilePath 为空时仅输出到 stdout，否则同时输出到 stdout 和指定文件。
func New(levelStr, logFilePath string) (*zap.Logger, error) {
	// 解析日志级别
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(levelStr)); err != nil {
		return nil, err
	}

	// Encoder 配置：console 输出，ISO8601 时间格式
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoder := zapcore.NewConsoleEncoder(encoderCfg)

	// 构建 WriteSyncer 列表
	syncers := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	if logFilePath != "" {
		// 打开或创建文件
		f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			// 如果打开文件失败，退回至 stdout
			zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), lvl)
		} else {
			syncers = append(syncers, zapcore.AddSync(f))
		}
	}

	// MultiWriteSyncer
	ws := zapcore.NewMultiWriteSyncer(syncers...)

	// 创建 core
	core := zapcore.NewCore(encoder, ws, lvl)

	// 包含 caller 信息
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return logger, nil
}
