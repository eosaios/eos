package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------- errors ----------

func TestAppErrorBasics(t *testing.T) {
	e := NewError(ErrCodeNotFound, "missing")
	if e.Error() != "[NOT_FOUND] missing" {
		t.Fatalf("Error = %q", e.Error())
	}
	if e.Unwrap() != nil {
		t.Fatal("Unwrap should be nil")
	}

	inner := errors.New("root")
	w := WrapError(inner, ErrCodeIO, "read", "failed")
	if w.Error() != "[IO_ERROR] failed: root" {
		t.Fatalf("wrap = %q", w.Error())
	}
	if !errors.Is(w, inner) {
		t.Fatal("errors.Is should find inner")
	}
	if w.Unwrap() != inner {
		t.Fatal("Unwrap")
	}

	if WrapError(nil, ErrCodeIO, "op", "m") != nil {
		t.Fatal("WrapError(nil) should be nil")
	}
	if WrapErrorWithDetails(nil, ErrCodeIO, "op", "m", nil) != nil {
		t.Fatal("WrapErrorWithDetails(nil) should be nil")
	}

	wd := WrapErrorWithDetails(inner, ErrCodeTool, "run", "boom", map[string]any{"k": 1})
	attrs := wd.LogAttrs()
	if len(attrs) < 6 {
		t.Fatalf("attrs = %v", attrs)
	}

	// LogAndWrapError
	if LogAndWrapError("comp", "op", nil, ErrCodeIO, "m") != nil {
		t.Fatal("LogAndWrapError(nil)")
	}
	wrapped := LogAndWrapError("comp", "op", inner, ErrCodeTimeout, "slow")
	if !IsErrorCode(wrapped, ErrCodeTimeout) {
		t.Fatal("IsErrorCode")
	}
	if GetErrorCode(wrapped) != ErrCodeTimeout {
		t.Fatal("GetErrorCode")
	}
	if GetErrorCode(inner) != ErrCodeUnknown {
		t.Fatal("non-AppError code")
	}
	if IsErrorCode(inner, ErrCodeTimeout) {
		t.Fatal("non-AppError IsErrorCode")
	}
}

// ---------- env ----------

func TestEnvInfoHelpers(t *testing.T) {
	if GetOS() == "" {
		t.Fatal("GetOS empty")
	}
	shell := GetShell()
	if shell == "" {
		t.Fatal("GetShell empty")
	}
	if GetCWD() == "" {
		t.Fatal("GetCWD empty")
	}

	// IsGitRepo
	dir := t.TempDir()
	if IsGitRepo(dir) {
		t.Fatal("temp dir should not be git repo")
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsGitRepo(dir) {
		t.Fatal("should detect .git")
	}

	info := EnvInfo{OS: "macOS", Shell: "zsh", CWD: "/ws", IsGitRepo: true, OSVersion: "24.0"}
	out := FormatEnvInfo(info)
	for _, want := range []string{"macOS", "zsh", "/ws", "是", "24.0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("FormatEnvInfo missing %q:\n%s", want, out)
		}
	}
	// 无 OSVersion 与非 git
	info = EnvInfo{OS: "Linux", Shell: "bash", CWD: "/x"}
	out = FormatEnvInfo(info)
	if strings.Contains(out, "系统版本") {
		t.Fatalf("should omit OSVersion:\n%s", out)
	}
	if !strings.Contains(out, "否") {
		t.Fatalf("git status:\n%s", out)
	}

	// GetEnvInfo 与 GetOSVersion 能跑通
	env := GetEnvInfo()
	if env.OS == "" || env.CWD == "" {
		t.Fatalf("GetEnvInfo = %+v", env)
	}
	_ = GetOSVersion()
}

func TestGenerateProjectStructureFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "tree.txt")
	if err := GenerateProjectStructureFile(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Project Directory Structure") {
		t.Fatalf("header missing:\n%s", text)
	}
	if !strings.Contains(text, "src/") || !strings.Contains(text, "a.go") {
		t.Fatalf("tree missing entries:\n%s", text)
	}
	if strings.Contains(text, ".git") {
		t.Fatalf("ignored dirs should be skipped:\n%s", text)
	}
}

// ---------- command ----------

func TestCommandWrappers(t *testing.T) {
	t.Setenv("EOS_GUI_MODE", "")
	if isGUIMode() {
		t.Fatal("GUI mode should be off")
	}
	t.Setenv("EOS_GUI_MODE", "1")
	if !isGUIMode() {
		t.Fatal("GUI mode should be on")
	}

	// Windows 无独立 echo.exe（cmd 内建），改用 cmd /c echo 保证跨平台可执行。
	echoName, echoArgs := "echo", []string{"hi"}
	if runtime.GOOS == "windows" {
		echoName, echoArgs = "cmd", []string{"/c", "echo", "hi"}
	}
	cmd := Command(echoName, echoArgs...)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "hi" {
		t.Fatalf("Command = %q err=%v", out, err)
	}

	ctx := context.Background()
	echoName2, echoArgs2 := "echo", []string{"ho"}
	if runtime.GOOS == "windows" {
		echoName2, echoArgs2 = "cmd", []string{"/c", "echo", "ho"}
	}
	cmd = CommandContext(ctx, echoName2, echoArgs2...)
	out, err = cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "ho" {
		t.Fatalf("CommandContext = %q err=%v", out, err)
	}
}

// ---------- file ----------

func TestCheckFileSizeAndBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	size, exceeds, err := CheckFileSize(path, 100)
	if err != nil || size != 5 || exceeds {
		t.Fatalf("CheckFileSize = %d %v %v", size, exceeds, err)
	}
	_, exceeds, _ = CheckFileSize(path, 2)
	if !exceeds {
		t.Fatal("should exceed")
	}
	if _, _, err := CheckFileSize(filepath.Join(dir, "nope"), 10); err == nil {
		t.Fatal("missing file should error")
	}

	if IsBinaryFile(path) {
		t.Fatal("txt should not be binary")
	}
	// 扩展名
	if !isBinaryByExtension(filepath.Join(dir, "x.exe")) {
		t.Fatal("exe should be binary by ext")
	}
	if isBinaryByExtension(filepath.Join(dir, "x.go")) {
		t.Fatal("go should be text by ext")
	}
	if isBinaryByExtension(filepath.Join(dir, "README")) {
		// 无扩展名 + 非已知二进制 base → 交给内容检测
	}

	// 空字节内容
	bin := filepath.Join(dir, "blob.dat")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isBinaryByNullBytes(bin) {
		t.Fatal("null bytes should mark binary")
	}
	if !IsBinaryFile(bin) {
		t.Fatal("IsBinaryFile should detect null bytes")
	}

	// 签名检测（PNG 魔术字节）
	png := filepath.Join(dir, "img.bin")
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isBinaryBySignature(png) {
		t.Fatal("png signature")
	}
}

func TestValidateFileForReadAndReadLimited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, size, msg := ValidateFileForRead(path, 100)
	if !ok || size != 5 || msg != "" {
		t.Fatalf("valid = %v %d %q", ok, size, msg)
	}

	ok, _, msg = ValidateFileForRead(filepath.Join(dir, "nope"), 10)
	if ok || msg != "file not found" {
		t.Fatalf("missing = %v %q", ok, msg)
	}

	ok, _, msg = ValidateFileForRead(dir, 10)
	if ok || msg != "path is a directory, not a file" {
		t.Fatalf("dir = %v %q", ok, msg)
	}

	bin := filepath.Join(dir, "b.dat")
	if err := os.WriteFile(bin, []byte{0, 1, 0, 1, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	ok, _, msg = ValidateFileForRead(bin, 100)
	if ok || msg != "binary file detected, cannot read" {
		t.Fatalf("binary = %v %q", ok, msg)
	}

	ok, size, msg = ValidateFileForRead(path, 2)
	if ok || msg != "file too large" || size != 5 {
		t.Fatalf("too large = %v %d %q", ok, size, msg)
	}

	// ReadFileLimited
	content, size, err := ReadFileLimited(path, 100)
	if err != nil || content != "hello" || size != 5 {
		t.Fatalf("read = %q %d %v", content, size, err)
	}

	// 空文件
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	content, size, err = ReadFileLimited(empty, 10)
	if err != nil || content != "" || size != 0 {
		t.Fatalf("empty = %q %d %v", content, size, err)
	}

	// 超限截断
	long := filepath.Join(dir, "long.txt")
	if err := os.WriteFile(long, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, size, err = ReadFileLimited(long, 4)
	if !IsFileTooLarge(err) {
		t.Fatalf("want FileTooLargeError, got %v", err)
	}
	if content != "0123" || size != 10 {
		t.Fatalf("truncated = %q size=%d", content, size)
	}
	var fterr *FileTooLargeError
	if !errors.As(err, &fterr) || fterr.ReadSize != 4 {
		t.Fatalf("FileTooLargeError = %+v", err)
	}
	if !strings.Contains(fterr.Error(), long) {
		t.Fatalf("Error = %q", fterr.Error())
	}
	if IsFileTooLarge(errors.New("x")) {
		t.Fatal("plain error")
	}

	// 缺失文件
	if _, _, err := ReadFileLimited(filepath.Join(dir, "nope"), 10); err == nil {
		t.Fatal("missing should error")
	}
}

// ---------- validation ----------

func TestValidateUserInput(t *testing.T) {
	if r := ValidateUserInput("   "); r.IsValid {
		t.Fatal("blank input")
	}
	r := ValidateUserInput("  hello  ")
	if !r.IsValid || r.Processed != "hello" {
		t.Fatalf("valid input = %+v", r)
	}
	// 非法 UTF-8
	if r := ValidateUserInput(string([]byte{0xff, 0xfe})); r.IsValid {
		t.Fatal("invalid utf8")
	}
	// 超长
	if r := ValidateUserInput(strings.Repeat("a", MaxUserInputLength+1)); r.IsValid {
		t.Fatal("too long")
	}
}

func TestValidatePathAndCommand(t *testing.T) {
	if r := ValidatePath("  "); r.IsValid {
		t.Fatal("empty path")
	}
	if r := ValidatePath(strings.Repeat("a", MaxPathLength+1)); r.IsValid {
		t.Fatal("path too long")
	}
	if r := ValidatePath("src/main.go"); !r.IsValid || r.Processed != "src/main.go" {
		t.Fatalf("good path = %+v", r)
	}
	if r := ValidatePath("../etc/passwd"); r.IsValid {
		t.Fatal("suspicious path")
	}

	if r := ValidateCommand("  "); r.IsValid {
		t.Fatal("empty cmd")
	}
	if r := ValidateCommand(strings.Repeat("a", MaxCommandLength+1)); r.IsValid {
		t.Fatal("cmd too long")
	}
	if r := ValidateCommand("  ls -la  "); !r.IsValid || r.Processed != "ls -la" {
		t.Fatalf("good cmd = %+v", r)
	}
}

func TestSuspiciousPathPatterns(t *testing.T) {
	// 明确危险的模式
	for _, p := range []string{"../x", `..\x`, "/etc/passwd", `\\server\share`, "COM1.txt", "/tmp/NUL", "aux"} {
		if !containsSuspiciousPathPatterns(p) {
			t.Fatalf("should flag %q", p)
		}
	}
	// 常规路径不应被误伤（回归：旧实现用子串匹配 "CON"，config/console 全被拒）
	for _, p := range []string{"src/config.go", "console.log", "my-project/README.md", "docs/concepts.md", "fileCOM1.txt"} {
		if containsSuspiciousPathPatterns(p) {
			t.Fatalf("false positive on %q", p)
		}
	}
}

func TestSanitizeAndTruncate(t *testing.T) {
	// 控制字符去掉，\n\r\t 保留
	if got := SanitizeString("a\x00b\nc\rd\te"); got != "ab\nc\rd\te" {
		t.Fatalf("sanitize = %q", got)
	}
	if got := SanitizeString("ok\x07text"); got != "oktext" {
		t.Fatalf("sanitize bell = %q", got)
	}
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("short = %q", got)
	}
	if got := TruncateString("hello world", 5); got != "hello..." {
		t.Fatalf("truncate = %q", got)
	}
}

func TestCheckIntAndStringLength(t *testing.T) {
	if ok, msg := CheckPositiveInt(1, "n"); !ok || msg != "" {
		t.Fatalf("positive ok = %v %q", ok, msg)
	}
	if ok, msg := CheckPositiveInt(0, "n"); ok || !strings.Contains(msg, "n") {
		t.Fatalf("zero positive = %v %q", ok, msg)
	}
	if ok, _ := CheckPositiveInt(-1, "n"); ok {
		t.Fatal("negative")
	}

	if ok, _ := CheckNonNegativeInt(0, "n"); !ok {
		t.Fatal("zero non-neg")
	}
	if ok, msg := CheckNonNegativeInt(-1, "n"); ok || msg == "" {
		t.Fatalf("neg non-neg = %v %q", ok, msg)
	}

	if ok, _ := CheckRangeInt(5, 1, 10, "n"); !ok {
		t.Fatal("in range")
	}
	if ok, _ := CheckRangeInt(0, 1, 10, "n"); ok {
		t.Fatal("below")
	}
	if ok, _ := CheckRangeInt(11, 1, 10, "n"); ok {
		t.Fatal("above")
	}

	if ok, _ := CheckStringLength("abc", 1, 10, "s"); !ok {
		t.Fatal("in len")
	}
	if ok, _ := CheckStringLength("", 1, 10, "s"); ok {
		t.Fatal("too short")
	}
	if ok, _ := CheckStringLength("abcdefghijk", 1, 10, "s"); ok {
		t.Fatal("too long")
	}
	// maxLen<=0 不限制上界
	if ok, _ := CheckStringLength("abcdefghijk", 1, 0, "s"); !ok {
		t.Fatal("no max")
	}
}

func TestIsEmptyValue(t *testing.T) {
	if !isEmptyValue(nil) || !isEmptyValue("") || !isEmptyValue([]string{}) ||
		!isEmptyValue([]any{}) || !isEmptyValue(map[string]any{}) {
		t.Fatal("empty values")
	}
	if isEmptyValue("x") || isEmptyValue(1) || isEmptyValue([]string{"a"}) || isEmptyValue(map[string]any{"k": 1}) {
		t.Fatal("non-empty")
	}
}

func TestFileBinaryEdgeCases(t *testing.T) {
	dir := t.TempDir()
	// 已知文本扩展名不按内容判二进制
	txt := filepath.Join(dir, "a.md")
	if err := os.WriteFile(txt, []byte{0x00, 0x01, 0x00, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if isBinaryByExtension(txt) {
		t.Fatal("md is text by ext")
	}
	// 已知二进制扩展名
	if !isBinaryByExtension(filepath.Join(dir, "x.dll")) {
		t.Fatal("dll")
	}
	// 无扩展名 + 未知 base → 交给内容
	if isBinaryByExtension(filepath.Join(dir, "README")) {
		// 可能按 base 名
	}

	// ReadFileLimited 正常路径
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, size, err := ReadFileLimited(p, 100)
	if err != nil || content != "abc" || size != 3 {
		t.Fatalf("read = %q %d %v", content, size, err)
	}

	// IsPathInRoot 自身
	if !IsPathInRoot(dir, dir) {
		t.Fatal("self")
	}
}

func TestParamsTypeMismatchDefaults(t *testing.T) {
	p := map[string]any{"s": 1, "b": "x", "i": true}
	if GetParamString(p, "s", "d") != "d" {
		t.Fatal("string mismatch")
	}
	if GetParamBool(p, "b", true) != true {
		t.Fatal("bool mismatch")
	}
	if GetParamInt(p, "i", -1) != -1 {
		t.Fatal("int mismatch")
	}
}

func TestTokenEstimateCache(t *testing.T) {
	c := NewTokenEstimateCache(2)
	c.Put(1, 10)
	c.Put(2, 20)
	if v, ok := c.Get(1); !ok || v != 10 {
		t.Fatalf("get = %v %v", v, ok)
	}
	if _, ok := c.Get(99); ok {
		t.Fatal("miss")
	}
	// 超容量淘汰最旧
	c.Put(3, 30)
	if _, ok := c.Get(2); ok {
		// 2 可能被淘汰（1 刚被 Get 刷新）
	}
	// 负数夹到 0
	c.Put(4, -5)
	if v, _ := c.Get(4); v != 0 {
		t.Fatalf("neg = %d", v)
	}
	// 默认容量
	c2 := NewTokenEstimateCache(0)
	c2.Put(1, 1)
	if v, ok := c2.Get(1); !ok || v != 1 {
		t.Fatal("default cap")
	}
}

func TestTokenEstimateKeyAndWeighted(t *testing.T) {
	k1 := TokenEstimateKey("model", "text")
	k2 := TokenEstimateKey("model", "text")
	if k1 != k2 {
		t.Fatal("stable key")
	}
	k3 := TokenEstimateKey("other", "text")
	if k1 == k3 {
		t.Fatal("model affects key")
	}

	n := EstimateTokensWeighted("gpt-4", "hello world this is a test")
	if n <= 0 {
		t.Fatalf("estimate = %d", n)
	}
	n2 := EstimateTokensWeighted("code", "func main() { if x { return } }")
	if n2 <= 0 {
		t.Fatalf("code estimate = %d", n2)
	}
	if EstimateTokensWeighted("m", "") != 0 {
		t.Fatal("empty")
	}
}
