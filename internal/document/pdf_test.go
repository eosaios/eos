package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"path/filepath"
	"testing"
)

func TestWriteAndReadPDFRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pdf")

	model := DocumentModel{
		Title: "Hello PDF",
		Blocks: []DocumentBlock{
			{Type: BlockHeading, Text: "Section", Level: 1},
			{Type: BlockParagraph, Text: "Body text."},
			{Type: BlockHeading, Text: "Sub", Level: 2},
			{Type: BlockTable, Rows: [][]string{{"a", "b"}, {"1", "2"}}},
			{Type: BlockHeading, Text: "Deep", Level: 3},
		},
	}
	if err := WritePDF(path, model); err != nil {
		t.Fatal(err)
	}

	got, err := ReadPDF(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata["source_format"] != "pdf" {
		t.Fatalf("meta = %v", got.Metadata)
	}
	// 正文应能抽出若干文字
	if len(got.Blocks) == 0 && got.PlainText() == "" {
		t.Fatalf("empty pdf content: %+v", got)
	}
}

func TestWritePDFEmptyTableAndReadMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-table.pdf")
	model := DocumentModel{
		Blocks: []DocumentBlock{
			{Type: BlockTable, Rows: nil},
			{Type: BlockParagraph, Text: "x"},
		},
	}
	if err := WritePDF(path, model); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadPDF(filepath.Join(dir, "nope.pdf")); err == nil {
		t.Fatal("missing pdf")
	}
}
