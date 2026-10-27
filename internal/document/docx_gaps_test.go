package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// docx 余臂批测：Read 错误族（非 zip/缺 document.xml/坏 XML）、表格与
// 标题/加粗/斜体解析、Write 的目录与创建错误、往返。

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZipDoc(t *testing.T, name string, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for k, v := range entries {
		w, _ := zw.Create(k)
		_, _ = w.Write([]byte(v))
	}
	zw.Close()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadDOCXErrorFamily(t *testing.T) {
	// 非 zip 文件。
	notZip := filepath.Join(t.TempDir(), "a.docx")
	os.WriteFile(notZip, []byte("plain text"), 0o644)
	if _, err := ReadDOCX(notZip); err == nil || !strings.Contains(err.Error(), "failed to open docx") {
		t.Fatalf("非 zip = %v", err)
	}
	// zip 但缺 document.xml。
	missing := writeZipDoc(t, "b.docx", map[string]string{"other.txt": "x"})
	if _, err := ReadDOCX(missing); err == nil || !strings.Contains(err.Error(), "missing word/document.xml") {
		t.Fatalf("缺正文 = %v", err)
	}
	// 坏 XML。
	bad := writeZipDoc(t, "c.docx", map[string]string{"word/document.xml": "<w:document><bad"})
	if _, err := ReadDOCX(bad); err == nil || !strings.Contains(err.Error(), "parse docx xml") {
		t.Fatalf("坏 XML = %v", err)
	}
}

func TestReadDOCXRichContent(t *testing.T) {
	xml := `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>二级标题</w:t></w:r></w:p>
<w:p><w:r><w:rPr><w:b/><w:i/></w:rPr><w:t>粗斜体</w:t></w:r></w:p>
<w:p><w:r><w:br/></w:r><w:r><w:t>换行后</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:p><w:r><w:t></w:t></w:r></w:p>
</w:body>
</w:document>`
	path := writeZipDoc(t, "rich.docx", map[string]string{"word/document.xml": xml})
	model, err := ReadDOCX(path)
	if err != nil {
		t.Fatalf("读取 = %v", err)
	}
	if model.Metadata["source_format"] != "docx" {
		t.Fatalf("metadata = %v", model.Metadata)
	}
	foundHeading, foundBold, foundTable := false, false, false
	for _, b := range model.Blocks {
		if b.Type == BlockHeading && strings.Contains(b.Text, "二级标题") {
			foundHeading = true
		}
		if strings.Contains(b.Text, "粗斜体") {
			foundBold = true
		}
		if b.Type == BlockTable && len(b.Rows) == 1 && len(b.Rows[0]) == 2 {
			foundTable = true
		}
	}
	if !foundHeading || !foundBold || !foundTable {
		t.Fatalf("内容解析 = %+v", model.Blocks)
	}

	// 往返：读出的模型再写出再读。
	out := filepath.Join(t.TempDir(), "round.docx")
	if err := WriteDOCX(out, model); err != nil {
		t.Fatalf("写出 = %v", err)
	}
	again, err := ReadDOCX(out)
	if err != nil {
		t.Fatalf("回读 = %v", err)
	}
	if len(again.Blocks) == 0 {
		t.Fatal("往返丢失内容")
	}
}

func TestWriteDOCXIOErrors(t *testing.T) {
	// 目录被文件占位。
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	if err := WriteDOCX(filepath.Join(blocker, "sub", "a.docx"), DocumentFromText("t", "c")); err == nil {
		t.Fatal("mkdir 失败应报错")
	}
	// create 失败：路径本身是目录。
	if err := WriteDOCX(t.TempDir(), DocumentFromText("t", "c")); err == nil {
		t.Fatal("目录作目标应报错")
	}
}
