package cli

// root_test.go — --default 工作区 flag 的单元测试。
//
// --default 的契约：打开时把进程 cwd 切到默认工作区（HOME/.eos/workspace），
// 与显式 --workspace 互斥（fail-fast），关闭时不产生任何副作用。

import (
	"os"
	"strings"
	"testing"

	"github.com/eosaios/eos/internal/config"
)

// withDefaultWorkspaceFlag 临时打开 --default 包级 flag，测试结束后恢复。
func withDefaultWorkspaceFlag(t *testing.T) {
	t.Helper()
	cliDefaultWorkspace = true
	t.Cleanup(func() { cliDefaultWorkspace = false })
}

// isolateHome 把 HOME/USERPROFILE 指向临时目录，隔离默认工作区路径，
// 并在测试结束后恢复原 cwd。
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	return home
}

// assertSameDir 断言 got 与 want 指向同一目录。cwd 与期望路径可能形态不同：
// macOS 下 /var 是 /private/var 的符号链接，Windows 下 os.Getwd 返回 8.3
// 短路径（golang/go#21373，如 RUNNER~1 vs runneradmin）。os.SameFile 按底层
// 文件身份比较，两种平台差异统一覆盖。
func assertSameDir(t *testing.T, got, want string) {
	t.Helper()
	fiGot, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	fiWant, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(fiGot, fiWant) {
		t.Fatalf("cwd = %q, want default workspace %q", got, want)
	}
}

func TestDefaultFlagRegisteredOnRoot(t *testing.T) {
	if f := rootCmd.PersistentFlags().Lookup("default"); f == nil {
		t.Fatal("missing --default persistent flag on root command")
	}
}

func TestApplyDefaultWorkspaceFlagSwitchesCWD(t *testing.T) {
	isolateHome(t)
	withDefaultWorkspaceFlag(t)

	// root 命令无 --workspace flag：--default 直接切换 cwd。
	if err := applyDefaultWorkspaceFlag(rootCmd); err != nil {
		t.Fatalf("applyDefaultWorkspaceFlag(rootCmd) error = %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	assertSameDir(t, wd, config.DefaultWorkspacePath())
}

func TestApplyDefaultWorkspaceFlagAppliesToSubcommandWithoutWorkspace(t *testing.T) {
	isolateHome(t)
	withDefaultWorkspaceFlag(t)

	// exec 定义了 --workspace 但未显式给出：--default 生效。
	if err := applyDefaultWorkspaceFlag(newExecCmd()); err != nil {
		t.Fatalf("applyDefaultWorkspaceFlag(execCmd) error = %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	assertSameDir(t, wd, config.DefaultWorkspacePath())
}

func TestApplyDefaultWorkspaceFlagRejectsExplicitWorkspace(t *testing.T) {
	withDefaultWorkspaceFlag(t)

	cmd := newExecCmd()
	if err := cmd.Flags().Set("workspace", "/tmp/proj"); err != nil {
		t.Fatal(err)
	}
	err := applyDefaultWorkspaceFlag(cmd)
	if err == nil {
		t.Fatal("applyDefaultWorkspaceFlag() = nil, want mutual-exclusion error")
	}
	if !strings.Contains(err.Error(), "--default") || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("error = %v, want --default/--workspace mutual exclusion", err)
	}
}

func TestApplyDefaultWorkspaceFlagDisabledKeepsCWD(t *testing.T) {
	cliDefaultWorkspace = false
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// flag 关闭：即使子命令带显式 --workspace 也直接放行（由命令自身处理）。
	cmd := newExecCmd()
	if err := cmd.Flags().Set("workspace", "/tmp/proj"); err != nil {
		t.Fatal(err)
	}
	if err := applyDefaultWorkspaceFlag(cmd); err != nil {
		t.Fatalf("applyDefaultWorkspaceFlag() error = %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if wd != origWd {
		t.Fatalf("cwd changed unexpectedly: %q -> %q", origWd, wd)
	}
}
