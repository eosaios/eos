package clip

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"strings"
	"testing"
)

func TestDIBToPNG_32bpp_TopDown(t *testing.T) {
	dib := make([]byte, 40+2*1*4)
	binary.LittleEndian.PutUint32(dib[0:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], uint32(int32(2)))
	var h int32 = -1
	binary.LittleEndian.PutUint32(dib[8:12], uint32(h))
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	binary.LittleEndian.PutUint32(dib[16:20], 0)

	pix := dib[40:]
	pix[0] = 0
	pix[1] = 0
	pix[2] = 255
	pix[3] = 0
	pix[4] = 0
	pix[5] = 255
	pix[6] = 0
	pix[7] = 0

	pngBytes, err := dibToPNG(dib)
	if err != nil {
		t.Fatalf("dibToPNG error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png decode error: %v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 1 {
		t.Fatalf("unexpected bounds: %v", img.Bounds())
	}
	r0, g0, b0, _ := img.At(0, 0).RGBA()
	if r0 < 0xff00 || g0 > 0x0100 || b0 > 0x0100 {
		t.Fatalf("expected red pixel at (0,0), got r=%x g=%x b=%x", r0, g0, b0)
	}
	r1, g1, b1, _ := img.At(1, 0).RGBA()
	if g1 < 0xff00 || r1 > 0x0100 || b1 > 0x0100 {
		t.Fatalf("expected green pixel at (1,0), got r=%x g=%x b=%x", r1, g1, b1)
	}
}

func TestDibToRGBAErors(t *testing.T) {
	if _, err := dibToRGBA([]byte{1, 2, 3}); err == nil {
		t.Fatal("too small")
	}
	// 头声明过大
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:4], 100)
	if _, err := dibToRGBA(hdr); err == nil {
		t.Fatal("invalid header size")
	}
	// 宽高非法
	hdr2 := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr2[0:4], 40)
	if _, err := dibToRGBA(hdr2); err == nil {
		t.Fatal("zero dims")
	}
	if _, err := dibToPNG([]byte{1}); err == nil {
		t.Fatal("png err")
	}
}

// buildDIB 构造最小合法 DIB（40 字节 BITMAPINFOHEADER + 像素数据）。
func buildDIB(w, h, bpp int, pixelBytes []byte, headerSize uint32, planes uint16, compression uint32) []byte {
	stride := ((bpp*w + 31) / 32) * 4
	out := make([]byte, int(headerSize)+stride*h)
	binary.LittleEndian.PutUint32(out[0:4], headerSize)
	binary.LittleEndian.PutUint32(out[4:8], uint32(w))
	binary.LittleEndian.PutUint32(out[8:12], uint32(h))
	binary.LittleEndian.PutUint16(out[12:14], planes)
	binary.LittleEndian.PutUint16(out[14:16], uint16(bpp))
	binary.LittleEndian.PutUint32(out[16:20], compression)
	copy(out[headerSize:], pixelBytes)
	return out
}

func TestDibToRGBADeepErrorArms(t *testing.T) {
	// planes != 1。
	dib := buildDIB(1, 1, 32, []byte{1, 2, 3, 4}, 40, 2, 0)
	if _, err := dibToRGBA(dib); err == nil || !strings.Contains(err.Error(), "planes") {
		t.Fatalf("planes error = %v", err)
	}
	// 非 BI_RGB 压缩。
	dib = buildDIB(1, 1, 32, []byte{1, 2, 3, 4}, 40, 1, 2)
	if _, err := dibToRGBA(dib); err == nil || !strings.Contains(err.Error(), "compression") {
		t.Fatalf("compression error = %v", err)
	}
	// bpp 不支持（16）。
	dib = buildDIB(1, 1, 16, []byte{0, 0}, 40, 1, 0)
	if _, err := dibToRGBA(dib); err == nil || !strings.Contains(err.Error(), "bitcount") {
		t.Fatalf("bitcount error = %v", err)
	}
	// 像素数据不足（声明 2 行但 buffer 只含 1 行像素）。
	dib = buildDIB(1, 2, 32, []byte{1, 2, 3, 4}, 40, 1, 0)
	dib = dib[:44] // make 预留 48 字节，截到 40+4 制造越界
	if _, err := dibToRGBA(dib); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("range error = %v", err)
	}
	// 宽为负。
	hdr := make([]byte, 44)
	binary.LittleEndian.PutUint32(hdr[0:4], 40)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(0xFFFFFFFD)) // -3 的补码
	binary.LittleEndian.PutUint32(hdr[8:12], 1)
	if _, err := dibToRGBA(hdr); err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("negative width error = %v", err)
	}
}

func TestDIB24bppBottomUp(t *testing.T) {
	// 24bpp、bottom-up（h>0）：文件首行像素渲染为图像底行（行序翻转）。
	// 24bpp 字节序 B,G,R：蓝行只点亮 B=0xff，红行只点亮 R=0xff。
	stride := 4 // ((24*1+31)/32)*4：3 字节像素对齐到 4 字节行
	pixels := make([]byte, stride*2)
	pixels[0] = 0xff        // 第一行 B → 蓝
	pixels[stride+2] = 0xff // 第二行 R → 红
	dib := buildDIB(1, 2, 24, pixels, 40, 1, 0)
	img, err := dibToRGBA(dib)
	if err != nil {
		t.Fatalf("dibToRGBA error = %v", err)
	}
	// bottom-up：图像顶行对应文件末行像素（红），底行对应文件首行（蓝）。
	r0, _, b0, _ := img.At(0, 0).RGBA()
	if r0 < 0xff00 || b0 > 0x0100 {
		t.Fatalf("top row should read last pixel row (red), got r=%x b=%x", r0, b0)
	}
	_, _, b1, _ := img.At(0, 1).RGBA()
	if b1 < 0xff00 {
		t.Fatalf("bottom row should read first pixel row (blue), got b=%x", b1)
	}
}
