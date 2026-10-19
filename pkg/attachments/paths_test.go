package attachments

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExtractTerminalFilePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "文件拖入 说明.md")
	second := filepath.Join(dir, "test$#().txt")
	for _, p := range []string{path, second} {
		if err := os.WriteFile(p, []byte("context"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	uri := (&url.URL{Scheme: "file", Path: uriPath}).String()
	cases := []struct {
		input, prompt string
		paths         []string
	}{
		{"'" + path + "'", "", []string{path}},
		{"\"" + path + "\"", "", []string{path}},
		{uri, "", []string{path}},
		{"'" + path + "' " + second, "", []string{path, second}},
		{"请检查 " + "'" + path + "'", "请检查", []string{path}},
		{"missing/path.md", "missing/path.md", nil},
		{dir, dir, nil},
		{"file://remote.example/etc/passwd", "file://remote.example/etc/passwd", nil},
		{"'" + path, "'" + path, nil},
		{"$(touch /tmp/should-not-run)", "$(touch /tmp/should-not-run)", nil},
	}
	// Windows terminal file paths use quoting; backslashes are separators.
	// POSIX shell escaping is exercised only on POSIX filesystem paths.
	if filepath.Separator != '\\' {
		cases = append(cases,
			struct {
				input, prompt string
				paths         []string
			}{strings.ReplaceAll(path, " ", "\\ ") + " ", "", []string{path}},
			struct {
				input, prompt string
				paths         []string
			}{strings.NewReplacer("$", "\\$", "#", "\\#", "(", "\\(", ")", "\\)").Replace(second), "", []string{second}},
		)
	}
	for _, tc := range cases {
		got, paths := ExtractPaths(tc.input)
		if got != tc.prompt || !reflect.DeepEqual(paths, tc.paths) {
			t.Errorf("%q => %q, %v; want %q, %v", tc.input, got, paths, tc.prompt, tc.paths)
		}
	}
}

func TestPathTokensPreserveWindowsSeparatorsAndDecodePOSIXEscapes(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\(docs)\note.md"`: `C:\Program Files\(docs)\note.md`,
		`C:\tmp\report.md`:                  `C:\tmp\report.md`,
		`/tmp/a\ b\$c.md`:                   `/tmp/a b$c.md`,
	}
	for input, want := range cases {
		tokens, ok := pathTokens(input)
		if !ok || len(tokens) != 1 || tokens[0].value != want {
			t.Fatalf("tokens(%q)=%+v, %v; want %q", input, tokens, ok, want)
		}
	}
}
