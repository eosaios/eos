package cli

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"bytes"
	"strings"
	"testing"
)

func TestDoctorReporter(t *testing.T) {
	var buf bytes.Buffer
	r := &doctorReporter{w: &buf}
	r.section("Sec")
	r.pass("ok %d", 1)
	r.warn("warn")
	r.fail("bad")
	r.fail("bad2")

	out := buf.String()
	if !strings.Contains(out, "Sec") || !strings.Contains(out, "[✓]") ||
		!strings.Contains(out, "[!]") || !strings.Contains(out, "[✗]") {
		t.Fatalf("out = %q", out)
	}
	if r.failed != 2 {
		t.Fatalf("failed = %d", r.failed)
	}
}

func TestRunDoctor(t *testing.T) {
	setTestHome(t)
	var buf bytes.Buffer
	err := runDoctor(&buf)
	// 本地占位内核可能让 doctor 报错，只要写出报告即可
	if !strings.Contains(buf.String(), "eos doctor") {
		t.Fatalf("report = %q", buf.String())
	}
	_ = err
}
