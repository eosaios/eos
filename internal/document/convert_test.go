package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Convert(src, ConversionOptions{TargetFormat: "pdf"}); err == nil {
		t.Fatal("unsupported source format must error")
	}
	docx := filepath.Join(dir, "ok.docx")
	if err := os.WriteFile(docx, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(docx, ConversionOptions{TargetFormat: "rtf"}); err == nil {
		t.Fatal("unsupported target format must error")
	}
	if _, err := Convert(docx, ConversionOptions{TargetFormat: "pdf", Fidelity: "ultra"}); err == nil {
		t.Fatal("unsupported fidelity must error")
	}
	if _, err := Convert(filepath.Join(dir, "missing.docx"), ConversionOptions{TargetFormat: "docx"}); err == nil {
		t.Fatal("missing source file must error")
	}
}

// 同格式转换走纯拷贝引擎，不解析内容。
func TestConvertSameFormatCopies(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "same.docx")
	content := []byte("not-a-real-docx-but-copy-does-not-parse")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Convert(src, ConversionOptions{TargetFormat: "docx", Fidelity: "content"})
	if err != nil {
		t.Fatalf("same-format convert: %v", err)
	}
	if result.UsedEngine != "copy" {
		t.Fatalf("UsedEngine = %q; want copy", result.UsedEngine)
	}
	data, err := os.ReadFile(result.DestinationPath)
	if err != nil {
		t.Fatalf("destination unreadable: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("copied content mismatch: %q", data)
	}
	if want := filepath.Join(dir, "same.docx"); result.DestinationPath != want {
		t.Fatalf("default destination = %q; want %q", result.DestinationPath, want)
	}
}

// 内容级链路全走纯 Go 读写器（不依赖 soffice）：
// 文本 → DOCX → 读回 → XLSX → 读回，逐级校验内容保真。
func TestConvertContentPipelineDocxXlsx(t *testing.T) {
	dir := t.TempDir()

	docxPath := filepath.Join(dir, "内容文档.docx")
	doc := DocumentFromText("季度报告", "第一季度营收增长\n\n第二季度利润回升")
	if err := WriteDOCX(docxPath, doc); err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	reread, err := ReadDOCX(docxPath)
	if err != nil {
		t.Fatalf("ReadDOCX: %v", err)
	}
	if plain := reread.PlainText(); !strings.Contains(plain, "第一季度营收增长") || !strings.Contains(plain, "第二季度利润回升") {
		t.Fatalf("docx round-trip lost content: %q", plain)
	}

	// docx → xlsx（content 引擎）。
	xlsxPath := filepath.Join(dir, "转出.xlsx")
	result, err := Convert(docxPath, ConversionOptions{DestinationPath: xlsxPath, TargetFormat: "xlsx", Fidelity: "content"})
	if err != nil {
		t.Fatalf("docx->xlsx: %v", err)
	}
	if result.UsedEngine != "content" {
		t.Fatalf("UsedEngine = %q; want content", result.UsedEngine)
	}
	book, err := ReadXLSX(xlsxPath)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	if plain := book.PlainText(); !strings.Contains(plain, "第一季度营收增长") {
		t.Fatalf("xlsx lost content: %q", plain)
	}

	// xlsx → docx（content 引擎）。
	backPath := filepath.Join(dir, "back.docx")
	if _, err := Convert(xlsxPath, ConversionOptions{DestinationPath: backPath, TargetFormat: "docx", Fidelity: "content"}); err != nil {
		t.Fatalf("xlsx->docx: %v", err)
	}
	finalDoc, err := ReadDOCX(backPath)
	if err != nil {
		t.Fatalf("ReadDOCX(back): %v", err)
	}
	if plain := finalDoc.PlainText(); !strings.Contains(plain, "第二季度利润回升") {
		t.Fatalf("round-trip lost content: %q", plain)
	}
}
