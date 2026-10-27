package webbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFileT(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newWorkspaceFilesBridge(t *testing.T) (*BridgeService, string) {
	t.Helper()
	root := t.TempDir()
	return &BridgeService{activeWorkspace: root}, root
}

func TestListWorkspaceDirectoryRoot(t *testing.T) {
	s, root := newWorkspaceFilesBridge(t)
	writeFileT(t, filepath.Join(root, "main.go"), "package main")
	writeFileT(t, filepath.Join(root, "zed.txt"), "")
	writeFileT(t, filepath.Join(root, "alpha", ".keep"), "")
	writeFileT(t, filepath.Join(root, "node_modules", "dep", "x.js"), "")
	writeFileT(t, filepath.Join(root, ".git", "config"), "")

	listing, err := s.ListWorkspaceDirectory("")
	if err != nil {
		t.Fatalf("ListWorkspaceDirectory: %v", err)
	}
	if listing.Path != root {
		t.Fatalf("Path = %q, want %q", listing.Path, root)
	}
	got := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		got = append(got, entry.Name)
	}
	// 目录优先 + 字母序；.git 与 node_modules 被忽略集过滤。
	want := []string{"alpha", "main.go", "zed.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestListWorkspaceDirectorySubdirAndDotRoot(t *testing.T) {
	s, root := newWorkspaceFilesBridge(t)
	writeFileT(t, filepath.Join(root, "alpha", "beta", "deep.md"), "# hi")

	for _, rel := range []string{"alpha", "alpha/beta", "./"} {
		listing, err := s.ListWorkspaceDirectory(rel)
		if err != nil {
			t.Fatalf("ListWorkspaceDirectory(%q): %v", rel, err)
		}
		if len(listing.Entries) == 0 {
			t.Fatalf("ListWorkspaceDirectory(%q) returned no entries", rel)
		}
	}

	listing, err := s.ListWorkspaceDirectory("alpha/beta")
	if err != nil {
		t.Fatalf("ListWorkspaceDirectory(alpha/beta): %v", err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "deep.md" || listing.Entries[0].IsDir {
		t.Fatalf("entries = %+v, want single file deep.md", listing.Entries)
	}
}

func TestListWorkspaceDirectoryRejectsEscape(t *testing.T) {
	s, root := newWorkspaceFilesBridge(t)
	if _, err := s.ListWorkspaceDirectory(".."); err == nil {
		t.Fatal("ListWorkspaceDirectory(..) should be rejected")
	}
	if _, err := s.ListWorkspaceDirectory(filepath.Join(root, "..", "elsewhere")); err == nil {
		t.Fatal("ListWorkspaceDirectory outside root should be rejected")
	}
}

func TestListWorkspaceDirectoryMissingDir(t *testing.T) {
	s, _ := newWorkspaceFilesBridge(t)
	if _, err := s.ListWorkspaceDirectory("no-such-dir"); err == nil {
		t.Fatal("ListWorkspaceDirectory(missing) should fail")
	}
}

func TestPreviewWorkspaceFileTruncatesAtLimit(t *testing.T) {
	s, root := newWorkspaceFilesBridge(t)
	line := strings.Repeat("x", 63) + "\n"
	big := strings.Repeat(line, (maxWorkspaceFilePreviewBytes/64)+8)
	writeFileT(t, filepath.Join(root, "big.log"), big)

	preview, err := s.PreviewWorkspaceFile("big.log", 0)
	if err != nil {
		t.Fatalf("PreviewWorkspaceFile: %v", err)
	}
	if !preview.Truncated {
		t.Fatal("Truncated = false, want true for oversized file")
	}
	if len(preview.Content) >= len(big) {
		t.Fatalf("content len = %d, want truncated below %d", len(preview.Content), len(big))
	}
	if !strings.HasSuffix(preview.Content, "\n") {
		t.Fatal("truncated content should end at a line boundary")
	}
}

func TestPreviewWorkspaceFileSmallFileNotTruncated(t *testing.T) {
	s, root := newWorkspaceFilesBridge(t)
	writeFileT(t, filepath.Join(root, "readme.md"), "# Title\n\n正文")
	preview, err := s.PreviewWorkspaceFile("readme.md", 3)
	if err != nil {
		t.Fatalf("PreviewWorkspaceFile: %v", err)
	}
	if preview.Truncated {
		t.Fatal("Truncated = true, want false")
	}
	if preview.Content != "# Title\n\n正文" {
		t.Fatalf("Content = %q", preview.Content)
	}
	if preview.Language != "markdown" || preview.Line != 3 {
		t.Fatalf("Language = %q, Line = %d", preview.Language, preview.Line)
	}
}

func TestLanguageFromPath(t *testing.T) {
	cases := map[string]string{
		"a.go": "go", "b.ts": "tsx", "c.tsx": "tsx", "d.jsx": "javascript",
		"e.json": "json", "f.jsonc": "json", "G.MD": "markdown",
		"lib.rs": "rust", "main.py": "python", "app.java": "java",
		"q.sql": "sql", "run.sh": "bash", "util.c": "c", "io.h": "c",
		"core.cpp": "cpp", "gem.hpp": "cpp", "web.rb": "ruby", "page.php": "php",
		"Makefile": "text", "data.bin": "text",
	}
	for path, want := range cases {
		if got := languageFromPath(path); got != want {
			t.Errorf("languageFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestJoinWorkspaceTreePath(t *testing.T) {
	cases := [][3]string{
		{"", "a", "a"},
		{".", "a", "a"},
		{"src", "main", "src/main"},
		{"src/main", "app.go", "src/main/app.go"},
	}
	for _, c := range cases {
		if got := joinWorkspaceTreePath(c[0], c[1]); got != c[2] {
			t.Errorf("joinWorkspaceTreePath(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}
