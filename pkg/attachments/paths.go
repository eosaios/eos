package attachments

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type pathToken struct {
	value      string
	start, end int
}

// ExtractPaths recognizes existing explicit local file paths in terminal input.
// Quoting and backslash escaping are decoded without evaluating shell syntax.
// Unrecognized text stays in the prompt, including missing paths and commands.
func ExtractPaths(text string) (string, []string) {
	tokens, ok := pathTokens(text)
	if !ok {
		return text, nil
	}
	var paths []string
	var remaining strings.Builder
	cursor := 0
	for _, token := range tokens {
		path := token.value
		if strings.HasPrefix(path, "file://") {
			u, err := url.Parse(path)
			if err != nil || (u.Host != "" && u.Host != "localhost") || u.RawQuery != "" || u.Fragment != "" {
				continue
			}
			path = filepath.FromSlash(u.Path)
		}
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			path = filepath.Join(home, path[2:])
		}
		if !filepath.IsAbs(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		remaining.WriteString(text[cursor:token.start])
		cursor = token.end
		paths = append(paths, filepath.Clean(path))
	}
	if len(paths) == 0 {
		return text, nil
	}
	remaining.WriteString(text[cursor:])
	return strings.TrimSpace(remaining.String()), paths
}

func pathTokens(text string) ([]pathToken, bool) {
	var tokens []pathToken
	for i := 0; i < len(text); {
		if pathSpace(text[i]) {
			i++
			continue
		}
		start := i
		prefix := i
		if text[prefix] == '\'' || text[prefix] == '"' {
			prefix++
		}
		windowsPath := prefix+2 < len(text) && text[prefix+1] == ':' && (text[prefix+2] == '\\' || text[prefix+2] == '/')
		var value strings.Builder
		var quote byte
		for i < len(text) {
			c := text[i]
			if quote == 0 && pathSpace(c) {
				break
			}
			if c == '\\' && quote != '\'' && !windowsPath {
				if i+1 >= len(text) {
					return nil, false
				}
				value.WriteByte(text[i+1])
				i += 2
				continue
			}
			if c == '\'' || c == '"' {
				if quote == 0 {
					quote = c
					i++
					continue
				}
				if quote == c {
					quote = 0
					i++
					continue
				}
			}
			value.WriteByte(c)
			i++
		}
		if quote != 0 {
			return nil, false
		}
		tokens = append(tokens, pathToken{value.String(), start, i})
	}
	return tokens, true
}

// Terminal shell delimiters are ASCII. Testing individual UTF-8 bytes with
// unicode.IsSpace splits valid Chinese filenames (e.g. 入 contains 0x85).
func pathSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
