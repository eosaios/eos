package ui

import (
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/eosaios/eos/internal/ui/views/shell"
)

func TestTerminalFilePasteQueuesAndPreservesDraft(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"
	app.shell.SetInputValue("检查这份文件")
	path := filepath.Join(t.TempDir(), "需求 说明.md")
	if err := os.WriteFile(path, []byte("# 目标\n实现拖放"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		app.Update(tea.PasteMsg{Content: "'" + path + "'"})
	}
	if app.shell.GetInputValue() != "检查这份文件" || !reflect.DeepEqual(app.pendingAttachmentPaths, []string{path}) {
		t.Fatalf("draft = %q, attachments = %v", app.shell.GetInputValue(), app.pendingAttachmentPaths)
	}
	prompt, files, err := app.prepareFileAttachments(app.shell.GetInputValue())
	if err != nil || prompt != "检查这份文件" || len(files) != 1 {
		t.Fatalf("prepare = %q, %+v, %v", prompt, files, err)
	}
	data, err := base64.StdEncoding.DecodeString(files[0].Path)
	if err != nil || string(data) != "# 目标\n实现拖放" || files[0].Name != "需求 说明.md" || files[0].Kind != "file" {
		t.Fatalf("payload = %+v, content = %q, %v", files[0], data, err)
	}
	app.handleGlobalKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(app.pendingAttachmentPaths) != 0 || app.shell.GetInputValue() != "" {
		t.Fatal("Esc did not clear draft attachments")
	}
}

func TestTerminalFilePasteLeavesBashAndProseAlone(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"
	app.shell.SetInputValue("")
	app.Update(tea.PasteMsg{Content: "ordinary text\nsecond line"})
	if app.shell.GetInputValue() != "ordinary text\nsecond line" {
		t.Fatalf("prose = %q", app.shell.GetInputValue())
	}
	path := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(path, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.shell.ClearInput()
	app.shell.ToggleMode()
	if app.shell.GetMode() != shell.ModeBash {
		t.Fatal("expected bash")
	}
	app.Update(tea.PasteMsg{Content: path})
	if app.shell.GetInputValue() != path || len(app.pendingAttachmentPaths) != 0 {
		t.Fatal("bash paste intercepted")
	}
}

func TestFileSendFailureKeepsDraftAndQueue(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"
	app.shell.SetInputValue("keep me")
	path := filepath.Join(t.TempDir(), "removed.md")
	app.pendingAttachmentPaths = []string{path}
	app.sendMessage()
	if app.shell.GetInputValue() != "keep me" || !reflect.DeepEqual(app.pendingAttachmentPaths, []string{path}) || app.state.Processing {
		t.Fatal("failed read consumed draft")
	}
}

func TestTerminalCopiedFileURLPasteQueuesMultipleFiles(t *testing.T) {
	setTestHome(t)
	app := newTestAppModel(t)
	app.activeView = "shell"
	app.shell.SetInputValue("原有说明")
	paths := []string{filepath.Join(t.TempDir(), "文件 拖入.md"), filepath.Join(t.TempDir(), "第二份.txt")}
	var clipboard string
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("copied content"), 0o600); err != nil {
			t.Fatal(err)
		}
		uriPath := filepath.ToSlash(path)
		if filepath.VolumeName(path) != "" {
			uriPath = "/" + uriPath
		}
		clipboard += (&url.URL{Scheme: "file", Path: uriPath}).String() + "\n"
	}
	app.Update(tea.PasteMsg{Content: clipboard})
	if app.shell.GetInputValue() != "原有说明" || !reflect.DeepEqual(app.pendingAttachmentPaths, paths) {
		t.Fatalf("draft=%q pending=%v", app.shell.GetInputValue(), app.pendingAttachmentPaths)
	}
}
