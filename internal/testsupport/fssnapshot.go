package testsupport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// FileEntry 是快照里的一条记录。
type FileEntry struct {
	// Rel 是相对根目录的路径，用 / 分隔，跨平台可比。
	Rel string
	// Mode 是权限位。
	Mode fs.FileMode
	// SHA256 是内容摘要；目录为 ""。
	SHA256 string
	// Size 是字节数；目录为 0。
	Size int64
	// IsDir 标记目录。
	IsDir bool
	// LinkTarget 是符号链接指向；非链接为 ""。
	LinkTarget string
}

// Snapshot 是某个目录树在某一时刻的完整状态。
type Snapshot map[string]FileEntry

// SnapshotDir 递归采集目录内容。
//
// 符号链接只记录链接本身与目标，不跟随 —— 跟随会把目录外的真实内容
// 混进快照，反而掩盖“越界写入”这类问题。
func SnapshotDir(t testing.TB, root string) Snapshot {
	t.Helper()
	out := Snapshot{}
	if err := walk(root, root, out); err != nil {
		t.Fatalf("testsupport: snapshot %s: %v", root, err)
	}
	return out
}

func walk(root, dir string, out Snapshot) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		full := filepath.Join(dir, entry.Name())
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				return err
			}
			out[rel] = FileEntry{Rel: rel, Mode: info.Mode(), LinkTarget: target}
		case entry.IsDir():
			out[rel] = FileEntry{Rel: rel, Mode: info.Mode(), IsDir: true}
			if err := walk(root, full, out); err != nil {
				return err
			}
		default:
			bs, err := os.ReadFile(full)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(bs)
			out[rel] = FileEntry{
				Rel:    rel,
				Mode:   info.Mode(),
				SHA256: hex.EncodeToString(sum[:]),
				Size:   info.Size(),
			}
		}
	}
	return nil
}

// Diff 返回“after 相对 before 新增或改变”的路径集合。
//
// 用于断言“副作用恰好发生一次”：只看 exists 检查会漏掉重复写入，
// 这里比较内容摘要，能发现写了两次导致内容变化的情况。
func (s Snapshot) Diff(after Snapshot) []string {
	var changed []string
	for rel, a := range after {
		b, ok := s[rel]
		if !ok {
			changed = append(changed, "created:"+rel)
			continue
		}
		if a.SHA256 != b.SHA256 {
			changed = append(changed, "content:"+rel)
			continue
		}
		if a.Mode != b.Mode {
			changed = append(changed, "mode:"+rel)
		}
	}
	for rel := range s {
		if _, ok := after[rel]; !ok {
			changed = append(changed, "removed:"+rel)
		}
	}
	sort.Strings(changed)
	return changed
}

// Created 返回相对 before 新建的路径。
func (s Snapshot) Created(after Snapshot) []string {
	var out []string
	for rel := range after {
		if _, ok := s[rel]; !ok {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// Format 返回适合失败信息的多行清单。
func (s Snapshot) Format() string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		e := s[k]
		switch {
		case e.IsDir:
			fmt.Fprintf(&b, "  dir  %s %v\n", k, e.Mode)
		case e.LinkTarget != "":
			fmt.Fprintf(&b, "  link %s -> %s\n", k, e.LinkTarget)
		default:
			fmt.Fprintf(&b, "  file %s %v sha256=%s size=%d\n", k, e.Mode, e.SHA256, e.Size)
		}
	}
	return b.String()
}

// SentinelUnchanged 断言工作区外的哨兵文件内容与权限都没有变化。
//
// 这是安全负例的核心检查：只断言 "err != nil" 无法证明没有越权写入。
func (e *Env) SentinelUnchanged(t testing.TB, before FileEntry) {
	t.Helper()
	after, err := os.Lstat(e.OutsideSentinel)
	if err != nil {
		t.Fatalf("SECURITY: 哨兵文件消失（%v）——说明工作区外的文件被删除了", err)
	}
	bs, err := os.ReadFile(e.OutsideSentinel)
	if err != nil {
		t.Fatalf("SECURITY: 哨兵文件不可读：%v", err)
	}
	sum := sha256.Sum256(bs)
	got := hex.EncodeToString(sum[:])
	if got != before.SHA256 {
		t.Fatalf("SECURITY: 哨兵文件内容被改写\nbefore sha256=%s\nafter  sha256=%s\ncontent=%q",
			before.SHA256, got, string(bs))
	}
	if after.Mode() != before.Mode {
		t.Fatalf("SECURITY: 哨兵文件权限被改写 before=%v after=%v", before.Mode, after.Mode())
	}
}

// OutsideUnchanged 断言整个工作区外目录树没有任何变化。
func (e *Env) OutsideUnchanged(t testing.TB, before Snapshot) {
	t.Helper()
	after := SnapshotDir(t, e.OutsideRoot)
	changed := before.Diff(after)
	if len(changed) > 0 {
		t.Fatalf("SECURITY: 工作区外目录发生变化：%v\nbefore:\n%s\nafter:\n%s",
			changed, before.Format(), after.Format())
	}
}

// SentinelEntry 返回哨兵文件的快照条目，供前后对比。
func (e *Env) SentinelEntry(t testing.TB) FileEntry {
	t.Helper()
	snap := SnapshotDir(t, e.OutsideRoot)
	return snap["sentinel.txt"]
}

// MustNotExist 断言路径不存在，并说明期望的失败原因。
func MustNotExist(t testing.TB, path, reason string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		info, _ := os.Stat(path)
		kind := "文件"
		if info != nil && info.IsDir() {
			kind = "目录"
		}
		t.Fatalf("SIDE-EFFECT: %s %s 不应存在（%s）", kind, path, reason)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

// MustBeDir 断言路径是目录，并返回其条目数。
func MustBeDir(t testing.TB, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("SIDE-EFFECT: 期望目录存在：%s（%v）", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("SIDE-EFFECT: %s 存在但不是目录", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("readdir %s: %v", path, err)
	}
	return len(entries)
}

// FileContent 读取文件内容，失败即 Fatal。
func FileContent(t testing.TB, path string) string {
	t.Helper()
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(bs)
}

// FileHash 返回文件 sha256，失败即 Fatal。
func FileHash(t testing.TB, path string) string {
	t.Helper()
	return sha256File(t, path)
}
