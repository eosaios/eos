package document

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"reflect"
	"strings"
	"testing"
)

func TestDocumentPlainText(t *testing.T) {
	doc := DocumentModel{
		Title: "  报告  ",
		Blocks: []DocumentBlock{
			{Type: BlockHeading, Text: "标题一", Level: 1},
			{Type: BlockParagraph, Text: "第一段"},
			{Type: BlockTable, Rows: [][]string{{"a", "b"}, {"c", "d"}}},
			{Type: BlockParagraph, Text: "   "}, // 空白段落应被跳过
		},
	}
	want := "报告\n\n标题一\n\n第一段\n\na | b\n\nc | d"
	if got := doc.PlainText(); got != want {
		t.Fatalf("PlainText() =\n%q\nwant\n%q", got, want)
	}
}

func TestWorkbookPlainTextAndFirstSheet(t *testing.T) {
	book := WorkbookModel{
		Title: "数据",
		Sheets: []WorkbookSheet{
			{Name: "汇总", Rows: [][]string{{"k", "v"}, {"n", "1"}}},
			{Name: "", Rows: [][]string{{"x"}}},
		},
	}
	want := "数据\n# Sheet: 汇总\nk\tv\nn\t1\n# Sheet: Sheet2\nx"
	if got := book.PlainText(); got != want {
		t.Fatalf("PlainText() =\n%q\nwant\n%q", got, want)
	}
	if got := book.FirstSheetName(); got != "汇总" {
		t.Fatalf("FirstSheetName() = %q; want 汇总", got)
	}
	if got := (WorkbookModel{}).FirstSheetName(); got != "Sheet1" {
		t.Fatalf("empty workbook FirstSheetName() = %q; want Sheet1", got)
	}
}

func TestNormalizeFormat(t *testing.T) {
	cases := map[string]string{
		"docx":        "docx",
		".DOCX":       "docx",
		"a/b/c.PDF":   "pdf",
		"report.XlSx": "xlsx",
		"md":          "",
		"":            "",
		"archive":     "",
	}
	for in, want := range cases {
		if got := NormalizeFormat(in); got != want {
			t.Fatalf("NormalizeFormat(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestDocumentFromTextSplitsParagraphs(t *testing.T) {
	doc := DocumentFromText("标题", "第一段\r\n续行\n\n第二段")
	want := DocumentModel{
		Title: "标题",
		Blocks: []DocumentBlock{
			{Type: BlockParagraph, Text: "第一段\n续行"},
			{Type: BlockParagraph, Text: "第二段"},
		},
	}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("DocumentFromText() = %+v; want %+v", doc, want)
	}

	// 只有标题时降级为单段落，保证文档非空。
	titleOnly := DocumentFromText("仅标题", "")
	if len(titleOnly.Blocks) != 1 || titleOnly.Blocks[0].Text != "仅标题" {
		t.Fatalf("title-only fallback = %+v", titleOnly.Blocks)
	}
}

func TestWorkbookFromText(t *testing.T) {
	book := WorkbookFromText("表", "行一\n\n行二\r\n行三")
	want := WorkbookModel{
		Title:  "表",
		Sheets: []WorkbookSheet{{Name: "Sheet1", Rows: [][]string{{"行一"}, {"行二"}, {"行三"}}}},
	}
	if len(book.Sheets) != 1 || book.Sheets[0].Name != want.Sheets[0].Name {
		t.Fatalf("sheets = %+v; want %+v", book.Sheets, want.Sheets)
	}
	if got, wantRows := book.Sheets[0].Rows, want.Sheets[0].Rows; len(got) != len(wantRows) {
		t.Fatalf("rows = %+v; want %+v", got, wantRows)
	}

	empty := WorkbookFromText("", "")
	if len(empty.Sheets) != 1 || len(empty.Sheets[0].Rows) != 1 {
		t.Fatalf("empty workbook must keep one placeholder row; got %+v", empty.Sheets)
	}
}

// 文档 ⇄ 工作簿模型互转：标题/警告/元数据保留，表格与标题映射到 sheet。
func TestDocumentWorkbookRoundTripModels(t *testing.T) {
	doc := DocumentModel{
		Title:    "T",
		Warnings: []string{"w1"},
		Metadata: map[string]string{"src": "test"},
		Blocks: []DocumentBlock{
			{Type: BlockHeading, Text: "S1", Level: 1},
			{Type: BlockTable, Rows: [][]string{{"a", "b"}}},
			{Type: BlockHeading, Text: "S2", Level: 1},
			{Type: BlockParagraph, Text: "p1\np2"},
		},
	}

	book := ToWorkbookModelFromDocument(doc)
	if book.Title != "T" || len(book.Warnings) != 1 || book.Metadata["src"] != "test" {
		t.Fatalf("workbook header fields = %+v", book)
	}
	if len(book.Sheets) != 2 {
		t.Fatalf("sheets = %+v; want 2 (heading 分 sheet)", book.Sheets)
	}
	if book.Sheets[0].Name != "S1" || len(book.Sheets[0].Rows) != 1 {
		t.Fatalf("sheet[0] = %+v", book.Sheets[0])
	}
	// 段落按行展开为行。
	if got := book.Sheets[1].Rows; len(got) != 2 || got[0][0] != "p1" || got[1][0] != "p2" {
		t.Fatalf("sheet[1].Rows = %+v; want p1/p2 两行", got)
	}

	back := ToDocumentModelFromWorkbook(book)
	if back.Title != "T" || len(back.Blocks) != 4 {
		t.Fatalf("round-trip blocks = %+v", back.Blocks)
	}
	if back.Blocks[0].Type != BlockHeading || back.Blocks[1].Type != BlockTable {
		t.Fatalf("round-trip block types = %+v", back.Blocks[:2])
	}
	if plain := back.PlainText(); !strings.Contains(plain, "S1") || !strings.Contains(plain, "a | b") {
		t.Fatalf("round-trip PlainText missing content: %q", plain)
	}
}
