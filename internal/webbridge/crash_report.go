package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

type CrashReport struct {
	Timestamp    time.Time     `json:"timestamp"`
	Panic        string        `json:"panic"`
	Stack        string        `json:"stack"`
	Build        BuildMetadata `json:"build"`
	Acknowledged bool          `json:"acknowledged"`
	Path         string        `json:"-"`
}

func crashReportDir() string {
	return filepath.Join(defaultLogDir(), "crashes")
}

func crashReportPath() string {
	return filepath.Join(crashReportDir(), "last_crash.json")
}

func WriteCrashReport(recovered any) (string, error) {
	report := CrashReport{
		Timestamp:    time.Now().UTC(),
		Panic:        fmt.Sprint(recovered),
		Stack:        string(debug.Stack()),
		Build:        CurrentBuildMetadata(),
		Acknowledged: false,
	}
	return saveCrashReport(report)
}

func LatestCrashReport() (*CrashReport, error) {
	return loadCrashReport()
}

func LoadPendingCrashReport() (*CrashReport, error) {
	report, err := loadCrashReport()
	if err != nil || report == nil {
		return report, err
	}
	if report.Acknowledged {
		return nil, nil
	}
	return report, nil
}

func MarkCrashReportAcknowledged() error {
	report, err := loadCrashReport()
	if err != nil || report == nil {
		return err
	}
	if report.Acknowledged {
		return nil
	}
	report.Acknowledged = true
	_, err = saveCrashReport(*report)
	return err
}

func loadCrashReport() (*CrashReport, error) {
	path := crashReportPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var report CrashReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	report.Path = path
	return &report, nil
}

func saveCrashReport(report CrashReport) (string, error) {
	path := crashReportPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
