// Package logging 提供 journald 友好的 JSON 日志。
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New 返回写到 stdout 的 JSON slog logger,级别来自配置字符串。
func New(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
