package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

// apply.go 余臂批测：ApplyWithClient 替换前安全臂（httptest 驱动）、
// downloadOnce 三态（200/206 续传/错误码）、downloadTo 取消与耗尽、
// verifyChecksum 三错、解压族（坏 zip/坏 gzip/tar 目录与穿越条目）、
// singleRootDir 空/多根。

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplyWithClientSafetyArms(t *testing.T) {
	ctx := context.Background()

	// 资产缺失三态。
	if _, err := ApplyWithClient(ctx, nil, nil, nil); err == nil {
		t.Fatal("nil res 应报错")
	}
	if _, err := ApplyWithClient(ctx, &CheckResult{AssetName: "a.tar.gz"}, nil, nil); err == nil {
		t.Fatal("缺 DownloadURL 应报错")
	}
	if _, err := ApplyWithClient(ctx, &CheckResult{DownloadURL: "u"}, nil, nil); err == nil {
		t.Fatal("缺 AssetName 应报错")
	}

	// 清单缺失：归档下载成功后拒绝。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer srv.Close()
	if _, err := ApplyWithClient(ctx, &CheckResult{
		DownloadURL: srv.URL, AssetName: "eos-cli_v1_darwin-arm64.tar.gz",
	}, nil, srv.Client()); err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("缺清单应拒绝: %v", err)
	}

	// 归档与清单齐全但校验不匹配：停在 replace 之前。
	if _, err := ApplyWithClient(ctx, &CheckResult{
		DownloadURL: srv.URL, AssetName: "eos-cli_v1_darwin-arm64.tar.gz",
		ChecksumURL: srv.URL + "/sums",
	}, nil, srv.Client()); err == nil {
		t.Fatal("坏归档/坏清单应报错")
	}
}

func TestApplyWithClientChecksumMismatch(t *testing.T) {
	tmp := t.TempDir()
	archivePath, assetName, wantSum := buildTestArchive(t, tmp, "eos", "eos-cli_v9.9.9_bad")
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	badSums := strings.Repeat("0", 64) + "  " + assetName + "\n"

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("GET /archive", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("GET /sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(badSums))
	})

	if _, err := ApplyWithClient(context.Background(), &CheckResult{
		DownloadURL: srv.URL + "/archive", AssetName: assetName,
		ChecksumURL: srv.URL + "/sums",
	}, nil, srv.Client()); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("校验不匹配应报错: %v", err)
	}
	_ = wantSum
}

func TestApplyWithClientBinaryMissing(t *testing.T) {
	tmp := t.TempDir()
	// 归档里没有 eos 二进制（只有别的文件）。
	archivePath, assetName, wantSum := buildTestArchive(t, tmp, "eos", "eos-cli_v9.9.9_bad")
	_ = wantSum
	archiveBytes, _ := os.ReadFile(archivePath)
	sums := wantSum + "  " + assetName + "\n"

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("GET /archive", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("GET /sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})

	// 直接喂校验通过的归档但改名资产，使解压后找不到 eos：
	// buildTestArchive 的二进制名固定 eos，这里通过换 root 名验证
	// 正常链在 stat 前的各错误臂已覆盖；binary-missing 用手写 zip 复现。
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("root/only-readme.txt")
	_, _ = w.Write([]byte("no binary"))
	_ = zw.Close()
	hashBytes := zbuf.Bytes()
	sumHex := sha256hex(hashBytes)

	mux2 := http.NewServeMux()
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	mux2.HandleFunc("GET /a.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(hashBytes)
	})
	mux2.HandleFunc("GET /sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumHex + "  a.zip\n"))
	})
	if _, err := ApplyWithClient(context.Background(), &CheckResult{
		DownloadURL: srv2.URL + "/a.zip", AssetName: "a.zip",
		ChecksumURL: srv2.URL + "/sums",
	}, nil, srv2.Client()); err == nil || !strings.Contains(err.Error(), "not found in archive") {
		t.Fatalf("归档缺二进制应报错: %v", err)
	}
}

func TestDownloadOnceThreeStates(t *testing.T) {
	body := "0123456789"
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("GET /full", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("GET /range", func(w http.ResponseWriter, r *http.Request) {
		rg := r.Header.Get("Range")
		if rg == "bytes=4-" {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(body[4:]))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("GET /err", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	dst := filepath.Join(t.TempDir(), "dl")
	// 200 全量。
	n, err := downloadOnce(context.Background(), srv.URL+"/full", dst, 0, nil, srv.Client())
	if err != nil || n != int64(len(body)) {
		t.Fatalf("200 = %d, %v", n, err)
	}
	// 进度回调路径。
	var reported int64
	if _, err := downloadOnce(context.Background(), srv.URL+"/full", dst, 0, func(received, total int64) {
		reported = received
	}, srv.Client()); err != nil {
		t.Fatal(err)
	}
	if reported == 0 {
		t.Fatal("进度未上报")
	}
	// 206 续传：先有 4 字节再续。
	if err := os.WriteFile(dst, []byte(body[:4]), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err = downloadOnce(context.Background(), srv.URL+"/range", dst, 4, nil, srv.Client())
	if err != nil || n != int64(len(body)-4) {
		t.Fatalf("206 = %d, %v", n, err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != body {
		t.Fatalf("续传结果 = %q", got)
	}
	// 非 2xx 错误码。
	if _, err := downloadOnce(context.Background(), srv.URL+"/err", dst, 0, nil, srv.Client()); err == nil {
		t.Fatal("500 应报错")
	}
	// 坏 URL 请求错误。
	if _, err := downloadOnce(context.Background(), "http://127.0.0.1:1/x", dst, 0, nil, srv.Client()); err == nil {
		t.Fatal("坏 URL 应报错")
	}
	// ctx 取消的 downloadTo。
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := downloadTo(cancelled, srv.URL+"/full", dst, nil, srv.Client()); err == nil {
		t.Fatal("取消 ctx 应报错")
	}
}

func TestVerifyChecksumErrors(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "a.zip")
	if err := os.WriteFile(archive, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 清单缺条目。
	sums := filepath.Join(tmp, "sums")
	os.WriteFile(sums, []byte(strings.Repeat("a", 64)+"  other.zip\n"), 0o644)
	if err := verifyChecksum(archive, sums, "a.zip"); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("缺条目 = %v", err)
	}
	// 清单不可读。
	if err := verifyChecksum(archive, filepath.Join(tmp, "missing"), "a.zip"); err == nil {
		t.Fatal("缺清单应报错")
	}
	// 归档不可读。
	os.WriteFile(sums, []byte(sha256hex([]byte("data"))+"  a.zip\n"), 0o644)
	if err := verifyChecksum(filepath.Join(tmp, "gone.zip"), sums, "a.zip"); err == nil {
		t.Fatal("缺归档应报错")
	}
}

func TestExtractArchiveFamily(t *testing.T) {
	dst := t.TempDir()
	// 坏 zip。
	badZip := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(badZip, []byte("not a zip"), 0o644)
	if err := extractArchive(badZip, dst); err == nil {
		t.Fatal("坏 zip 应报错")
	}
	// 坏 gzip。
	badTgz := filepath.Join(t.TempDir(), "bad.tar.gz")
	os.WriteFile(badTgz, []byte("not gzip"), 0o644)
	if err := extractArchive(badTgz, dst); err == nil {
		t.Fatal("坏 gzip 应报错")
	}
	// tar：目录条目 + 文件 + 穿越条目（被 safeJoin 拒绝跳过）。
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, h := range []tar.Header{
		{Name: "root/", Typeflag: tar.TypeDir},
		{Name: "root/file.txt", Typeflag: tar.TypeReg, Size: 3, Mode: 0o644},
		{Name: "../escape.txt", Typeflag: tar.TypeReg, Size: 3, Mode: 0o644},
	} {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			_, _ = tw.Write([]byte("abc"))
		}
	}
	tw.Close()
	gw.Close()
	tgz := filepath.Join(t.TempDir(), "ok.tar.gz")
	os.WriteFile(tgz, buf.Bytes(), 0o644)
	out := t.TempDir()
	if err := extractArchive(tgz, out); err != nil {
		t.Fatalf("tar.gz 解压: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "root", "file.txt")); err != nil {
		t.Fatal("tar 文件未落盘")
	}
	if _, err := os.Stat(filepath.Join(out, "escape.txt")); err == nil {
		t.Fatal("穿越条目不应落入解压目录")
	}
	// 单根判定：单目录包裹则进一层；空/多根/含文件均透传原目录。
	single := t.TempDir()
	os.MkdirAll(filepath.Join(single, "wrapped"), 0o755)
	if got, err := singleRootDir(single); err != nil || filepath.Base(got) != "wrapped" {
		t.Fatalf("单根 = %q, %v", got, err)
	}
	empty := t.TempDir()
	if got, err := singleRootDir(empty); err != nil || got != empty {
		t.Fatalf("空目录透传 = %q, %v", got, err)
	}
	multi := t.TempDir()
	os.MkdirAll(filepath.Join(multi, "a"), 0o755)
	os.MkdirAll(filepath.Join(multi, "b"), 0o755)
	if got, err := singleRootDir(multi); err != nil || got != multi {
		t.Fatalf("多根透传 = %q, %v", got, err)
	}
	withFile := t.TempDir()
	os.WriteFile(filepath.Join(withFile, "f"), []byte("x"), 0o644)
	if got, err := singleRootDir(withFile); err != nil || got != withFile {
		t.Fatalf("含文件透传 = %q, %v", got, err)
	}
	if _, err := singleRootDir(filepath.Join(multi, "gone")); err == nil {
		t.Fatal("目录不可读应报错")
	}
}

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// TestApplyWithClientFullSuccess 完整成功链：临时测试二进制被替换是
// 预期副作用（unix 上 rename 交换不影响运行中进程；windows 跳过——
// 运行中自替换语义不同）。core/ 落在测试二进制旁的临时构建目录。
func TestApplyWithClientFullSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows rename-in-place semantics differ")
	}
	tmp := t.TempDir()
	archivePath, assetName, wantSum := buildTestArchive(t, tmp, "eos", "eos-cli_v9.9.9_full")
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sums := wantSum + "  " + assetName + "\n"

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("GET /archive", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("GET /sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})

	outcome, err := ApplyWithClient(context.Background(), &CheckResult{
		DownloadURL: srv.URL + "/archive", AssetName: assetName,
		ChecksumURL: srv.URL + "/sums",
	}, nil, srv.Client())
	if err != nil {
		t.Fatalf("成功链: %v", err)
	}
	if outcome == nil || outcome.BinaryPath == "" || !outcome.CoreUpdated {
		t.Fatalf("outcome = %+v", outcome)
	}
	// 新 core 已落测试二进制旁。
	exePath, err := currentExecutable()
	if err != nil {
		t.Fatal(err)
	}
	newCore := filepath.Join(filepath.Dir(exePath), "core", "aarch64-test-triple", "eos-core")
	got, err := os.ReadFile(newCore)
	if err != nil || string(got) != "new-core" {
		t.Fatalf("core 未更新: %v %q", err, got)
	}
	// 二进制本体被替换为新内容。
	gotBin, err := os.ReadFile(exePath)
	if err != nil || string(gotBin) != "new-binary" {
		t.Fatalf("二进制未替换: %v", err)
	}
}

func TestExtractTarGzSpecialTypesAndConflicts(t *testing.T) {
	build := func(headers []tar.Header) string {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		for _, h := range headers {
			if err := tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			if h.Size > 0 {
				_, _ = tw.Write([]byte("data"))
			}
		}
		tw.Close()
		gw.Close()
		p := filepath.Join(t.TempDir(), "a.tar.gz")
		os.WriteFile(p, buf.Bytes(), 0o644)
		return p
	}
	// 非目录/常规类型（symlink 等）：静默跳过不落盘。
	p := build([]tar.Header{
		{Name: "root/", Typeflag: tar.TypeDir},
		{Name: "root/link", Typeflag: tar.TypeSymlink, Linkname: "target"},
		{Name: "root/unknown", Typeflag: tar.TypeBlock},
		{Name: "root/reg.txt", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644},
	})
	out := t.TempDir()
	if err := extractArchive(p, out); err != nil {
		t.Fatalf("特殊类型解压: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "root", "link")); !os.IsNotExist(err) {
		t.Fatal("symlink 不应落盘")
	}
	if _, err := os.Stat(filepath.Join(out, "root", "reg.txt")); err != nil {
		t.Fatal("常规文件应落盘")
	}
	// 文件冲突：归档把目录写到既有文件路径下 → MkdirAll 失败臂。
	blocker := t.TempDir()
	os.WriteFile(filepath.Join(blocker, "root"), []byte("x"), 0o644)
	p2 := build([]tar.Header{
		{Name: "root/", Typeflag: tar.TypeDir},
		{Name: "root/f", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644},
	})
	if err := extractArchive(p2, blocker); err == nil {
		t.Fatal("目录写至文件应报错")
	}
	// zip 同款冲突：条目父目录是文件。
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("blocked/child.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	zp := filepath.Join(t.TempDir(), "a.zip")
	os.WriteFile(zp, zbuf.Bytes(), 0o644)
	zblocker := t.TempDir()
	os.WriteFile(filepath.Join(zblocker, "blocked"), []byte("x"), 0o644)
	if err := extractArchive(zp, zblocker); err == nil {
		t.Fatal("zip 父目录冲突应报错")
	}
}
