package clip

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

const (
	biRGB = 0
)

func dibToPNG(dib []byte) ([]byte, error) {
	img, err := dibToRGBA(dib)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dibToRGBA(dib []byte) (*image.RGBA, error) {
	if len(dib) < 40 {
		return nil, errors.New("dib too small")
	}
	sz := binary.LittleEndian.Uint32(dib[0:4])
	if sz < 40 || int(sz) > len(dib) {
		return nil, errors.New("invalid dib header")
	}
	w := int(int32(binary.LittleEndian.Uint32(dib[4:8])))
	hRaw := int(int32(binary.LittleEndian.Uint32(dib[8:12])))
	if w <= 0 || hRaw == 0 {
		return nil, errors.New("invalid dib dimensions")
	}
	topDown := false
	h := hRaw
	if hRaw < 0 {
		topDown = true
		h = -hRaw
	}
	planes := binary.LittleEndian.Uint16(dib[12:14])
	if planes != 1 {
		return nil, errors.New("unsupported dib planes")
	}
	bpp := int(binary.LittleEndian.Uint16(dib[14:16]))
	comp := binary.LittleEndian.Uint32(dib[16:20])
	if comp != biRGB {
		return nil, errors.New("unsupported dib compression")
	}
	if bpp != 24 && bpp != 32 {
		return nil, errors.New("unsupported dib bitcount")
	}

	rowStride := ((bpp*w + 31) / 32) * 4
	pixOff := int(sz)
	if pixOff < 0 || pixOff+rowStride*h > len(dib) {
		return nil, errors.New("dib pixel data out of range")
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcY := y
		if !topDown {
			srcY = h - 1 - y
		}
		row := dib[pixOff+srcY*rowStride : pixOff+srcY*rowStride+rowStride]
		switch bpp {
		case 32:
			for x := 0; x < w; x++ {
				i := x * 4
				b := row[i]
				g := row[i+1]
				r := row[i+2]
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 0xff})
			}
		case 24:
			for x := 0; x < w; x++ {
				i := x * 3
				b := row[i]
				g := row[i+1]
				r := row[i+2]
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 0xff})
			}
		}
	}
	return img, nil
}
