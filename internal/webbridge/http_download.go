package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// downloadHTTPToFile 流式下载 url 到 destPath（覆盖写）。
// client 为 nil 时用默认客户端（遵循环境 HTTP_PROXY 约定）。
// totalHint 仅在服务端未返回 Content-Length 时作为总量提示（算进度百分比用，
// 传 0 表示未知）。失败时清理半成品文件，不留脏数据。
// 终端 shell 安装与应用内更新下载共用本实现（归一化，勿再复制流式循环）。
func downloadHTTPToFile(ctx context.Context, client *http.Client, url, destPath string, totalHint int64, progress func(received, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	active := client
	if active == nil {
		active = http.DefaultClient
	}
	resp, err := active.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	path := destPath
	total := resp.ContentLength
	if total <= 0 {
		total = totalHint
	}
	buf := make([]byte, 64*1024)
	var received int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				file.Close()
				_ = os.Remove(path)
				return writeErr
			}
			received += int64(n)
			if progress != nil {
				progress(received, total)
			}
		}
		if readErr != nil {
			file.Close()
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			_ = os.Remove(path)
			return readErr
		}
	}
}
