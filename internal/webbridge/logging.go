package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	loggerMu     sync.Mutex
	loggerOutput *lumberjack.Logger
)

func UsesWorkspaceState() bool {
	if override, ok := workspaceStateOverride(); ok {
		return override
	}
	return strings.TrimSpace(os.Getenv("FRONTEND_DEVSERVER_URL")) != ""
}

func workspaceStateOverride() (bool, bool) {
	raw, ok := os.LookupEnv("EOS_WORKSPACE_STATE")
	if !ok {
		return false, false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, false
	}
	if parsed, err := strconv.ParseBool(trimmed); err == nil {
		return parsed, true
	}
	switch strings.ToLower(trimmed) {
	case "on", "yes":
		return true, true
	case "off", "no":
		return false, true
	default:
		return false, false
	}
}

func DevStateDir() string {
	wd, err := os.Getwd()
	if err != nil || strings.TrimSpace(wd) == "" {
		home, _ := os.UserHomeDir()
		wd = home
	}
	return filepath.Join(wd, ".tmp", "eos-dev")
}

func defaultSystemLogDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(base) == "" {
			base = os.Getenv("APPDATA")
		}
		if strings.TrimSpace(base) == "" {
			base, _ = os.UserHomeDir()
		}
		return filepath.Join(base, "EOS", "logs")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "EOS")
	default:
		base := os.Getenv("XDG_STATE_HOME")
		if strings.TrimSpace(base) == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(base, "eos", "logs")
	}
}

func defaultLogDir() string {
	return configuredLogDir()
}

func logLevel() slog.Level {
	lvl := os.Getenv("EOS_LOG_LEVEL")
	if strings.TrimSpace(lvl) == "" {
		lvl = os.Getenv("LOG_LEVEL")
	}
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}

func initLogger() (string, error) {
	dir := filepath.Join(defaultLogDir(), "app")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	file := filepath.Join(dir, "eos-app.log")
	out := &lumberjack.Logger{
		Filename:   file,
		MaxSize:    10,
		MaxBackups: 7,
		MaxAge:     30,
		Compress:   true,
	}
	h := slog.NewJSONHandler(out, &slog.HandlerOptions{AddSource: true, Level: logLevel()})
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if loggerOutput != nil {
		_ = loggerOutput.Close()
	}
	loggerOutput = out
	slog.SetDefault(slog.New(h))
	return file, nil
}

func InitLogger() (string, error) {
	return initLogger()
}

func DefaultLogDir() string {
	return defaultLogDir()
}

func CloseLogger_FOR_TESTS_ONLY() {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if loggerOutput != nil {
		_ = loggerOutput.Close()
		loggerOutput = nil
	}
}
