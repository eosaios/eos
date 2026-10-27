package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

// 批十八批测：contentConvert 矩阵补齐（pdf 源 + 各格式→pdf）与不支持组合、
// Convert 校验臂、WriteXLSX 空/命名边界。
// convertWithSoffice 豁免：真 LibreOffice 子进程（本机无 soffice 走
// not found 分支已覆盖）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteSource(t *testing.T, dir, name string, write func(string) error) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := write(path); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestContentConvertMatrixToPDF(t *testing.T) {
	dir := t.TempDir()
	doc := DocumentFromText("报表", "上半年收入\n\n下半年支出")

	t.Run("docx→pdf", func(t *testing.T) {
		src := mustWriteSource(t, dir, "a.docx", func(p string) error { return WriteDOCX(p, doc) })
		dst := filepath.Join(dir, "a.pdf")
		if _, err := Convert(src, ConversionOptions{DestinationPath: dst, TargetFormat: "pdf", Fidelity: "content"}); err != nil {
			t.Fatalf("docx->pdf: %v", err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("pdf missing: %v", err)
		}
	})

	t.Run("xlsx→pdf", func(t *testing.T) {
		src := mustWriteSource(t, dir, "b.xlsx", func(p string) error {
			return WriteXLSX(p, WorkbookModel{Sheets: []WorkbookSheet{{Name: "数据", Rows: [][]string{{"项", "值"}, {"收入", "100"}}}}})
		})
		dst := filepath.Join(dir, "b.pdf")
		if _, err := Convert(src, ConversionOptions{DestinationPath: dst, TargetFormat: "pdf", Fidelity: "content"}); err != nil {
			t.Fatalf("xlsx->pdf: %v", err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("pdf missing: %v", err)
		}
	})

	t.Run("pdf→docx", func(t *testing.T) {
		src := mustWriteSource(t, dir, "c.pdf", func(p string) error { return WritePDF(p, doc) })
		dst := filepath.Join(dir, "c.docx")
		if _, err := Convert(src, ConversionOptions{DestinationPath: dst, TargetFormat: "docx", Fidelity: "content"}); err != nil {
			t.Fatalf("pdf->docx: %v", err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("docx missing: %v", err)
		}
	})
}

func TestContentConvertUnsupportedCombinations(t *testing.T) {
	dir := t.TempDir()
	// pdf→xlsx 在 content 矩阵内（九格全满），直接验证成功。
	src := mustWriteSource(t, dir, "x.pdf", func(p string) error {
		return WritePDF(p, DocumentFromText("t", "正文"))
	})
	dst := filepath.Join(dir, "x.xlsx")
	if _, err := Convert(src, ConversionOptions{DestinationPath: dst, TargetFormat: "xlsx", Fidelity: "content"}); err != nil {
		t.Fatalf("pdf->xlsx content = %v", err)
	}

	// default 分支：合法 srcFormat 但组合不在 switch（直调内部函数）。
	if err := contentConvert("", "", "pptx", "pdf"); err == nil || !strings.Contains(err.Error(), "unsupported conversion") {
		t.Fatalf("default branch = %v", err)
	}

	// 坏源文件（ReadPDF 失败透传）。
	badPath := filepath.Join(dir, "bad.pdf")
	if err := os.WriteFile(badPath, []byte("not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(badPath, ConversionOptions{TargetFormat: "docx", Fidelity: "content"}); err == nil {
		t.Fatal("corrupt source should fail content conversion")
	}
}

func TestConvertValidationArms(t *testing.T) {
	dir := t.TempDir()
	src := mustWriteSource(t, dir, "v.docx", func(p string) error {
		return WriteDOCX(p, DocumentFromText("t", "x"))
	})

	// 非法 fidelity。
	if _, err := Convert(src, ConversionOptions{TargetFormat: "pdf", Fidelity: "mid"}); err == nil || !strings.Contains(err.Error(), "unsupported fidelity") {
		t.Fatalf("bad fidelity = %v", err)
	}
	// 非法目标格式。
	if _, err := Convert(src, ConversionOptions{TargetFormat: "pptx"}); err == nil || !strings.Contains(err.Error(), "unsupported target format") {
		t.Fatalf("bad target = %v", err)
	}
	// 非法源格式。
	if _, err := Convert(filepath.Join(dir, "nope.pptx"), ConversionOptions{TargetFormat: "pdf"}); err == nil || !strings.Contains(err.Error(), "unsupported source format") {
		t.Fatalf("bad source = %v", err)
	}
}

func TestWriteXLSXNamingAndEmptyEdges(t *testing.T) {
	dir := t.TempDir()

	t.Run("空模型落默认 Sheet1", func(t *testing.T) {
		path := filepath.Join(dir, "empty.xlsx")
		if err := WriteXLSX(path, WorkbookModel{}); err != nil {
			t.Fatalf("WriteXLSX(empty) = %v", err)
		}
		book, err := ReadXLSX(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(book.Sheets) == 0 || book.Sheets[0].Name == "" {
			t.Fatalf("empty model should default Sheet1: %+v", book.Sheets)
		}
	})

	t.Run("多 sheet 与无名 sheet 默认命名", func(t *testing.T) {
		path := filepath.Join(dir, "multi.xlsx")
		model := WorkbookModel{Sheets: []WorkbookSheet{
			{Name: "一", Rows: [][]string{{"a"}}},
			{Rows: [][]string{{"b"}}}, // 无名 → Sheet2 兜底
		}}
		if err := WriteXLSX(path, model); err != nil {
			t.Fatalf("WriteXLSX(multi) = %v", err)
		}
		book, err := ReadXLSX(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(book.Sheets) != 2 {
			t.Fatalf("sheets = %d, want 2", len(book.Sheets))
		}
		if book.Sheets[0].Name != "一" {
			t.Fatalf("first sheet = %q", book.Sheets[0].Name)
		}
		if plain := book.PlainText(); !strings.Contains(plain, "a") || !strings.Contains(plain, "b") {
			t.Fatalf("content lost: %q", plain)
		}
	})

	t.Run("行内空单元格与空行兜底", func(t *testing.T) {
		path := filepath.Join(dir, "sparse.xlsx")
		model := WorkbookModel{Sheets: []WorkbookSheet{{Name: "s", Rows: [][]string{
			{"x", "", "y"},
			{},
		}}}}
		if err := WriteXLSX(path, model); err != nil {
			t.Fatalf("WriteXLSX(sparse) = %v", err)
		}
		if _, err := ReadXLSX(path); err != nil {
			t.Fatalf("ReadXLSX(sparse) = %v", err)
		}
	})

	t.Run("目标父目录自动创建", func(t *testing.T) {
		path := filepath.Join(dir, "nested", "deep", "out.xlsx")
		if err := WriteXLSX(path, WorkbookModel{Sheets: []WorkbookSheet{{Name: "s"}}}); err != nil {
			t.Fatalf("nested write = %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	})
}
