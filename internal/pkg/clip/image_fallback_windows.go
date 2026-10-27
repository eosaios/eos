//go:build windows

package clip

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	cfBitmap = 2
	cfDIB    = 8
	cfDIBV5  = 17
)

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard              = modUser32.NewProc("OpenClipboard")
	procCloseClipboard             = modUser32.NewProc("CloseClipboard")
	procIsClipboardFormatAvailable = modUser32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData           = modUser32.NewProc("GetClipboardData")
	procRegisterClipboardFormatW   = modUser32.NewProc("RegisterClipboardFormatW")

	procGlobalLock    = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock  = modKernel32.NewProc("GlobalUnlock")
	procGlobalSize    = modKernel32.NewProc("GlobalSize")
	procRtlMoveMemory = modKernel32.NewProc("RtlMoveMemory")
)

func readImageFallback() ([]byte, error) {
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return nil, errors.New("open clipboard failed")
	}
	defer func() { _, _, _ = procCloseClipboard.Call() }()

	if b, err := readClipboardPNG(); err == nil && len(b) > 0 {
		return b, nil
	}

	if b, err := readClipboardDIB(cfDIBV5); err == nil && len(b) > 0 {
		return b, nil
	}
	if b, err := readClipboardDIB(cfDIB); err == nil && len(b) > 0 {
		return b, nil
	}

	if _, err := readClipboardRaw(cfBitmap); err == nil {
		return nil, errors.New("clipboard bitmap format not supported yet")
	}
	return nil, errors.New("no supported image formats")
}

func readClipboardPNG() ([]byte, error) {
	name, _ := syscall.UTF16PtrFromString("PNG")
	f, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	if f == 0 {
		return nil, errors.New("register png format failed")
	}
	if r, _, _ := procIsClipboardFormatAvailable.Call(f); r == 0 {
		return nil, errors.New("png not available")
	}
	return readClipboardRaw(uint32(f))
}

func readClipboardDIB(format uint32) ([]byte, error) {
	if r, _, _ := procIsClipboardFormatAvailable.Call(uintptr(format)); r == 0 {
		return nil, errors.New("dib not available")
	}
	raw, err := readClipboardRaw(format)
	if err != nil {
		return nil, err
	}
	return dibToPNG(raw)
}

func readClipboardRaw(format uint32) ([]byte, error) {
	h, _, _ := procGetClipboardData.Call(uintptr(format))
	if h == 0 {
		return nil, errors.New("get clipboard data failed")
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil, errors.New("global lock failed")
	}
	defer func() { _, _, _ = procGlobalUnlock.Call(h) }()

	sz, _, _ := procGlobalSize.Call(h)
	if sz == 0 {
		return nil, errors.New("global size 0")
	}
	n := int(sz)
	b := make([]byte, n)
	if n > 0 {
		_, _, _ = procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&b[0])), p, uintptr(n))
	}
	return b, nil
}
