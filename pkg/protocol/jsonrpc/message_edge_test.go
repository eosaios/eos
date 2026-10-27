package jsonrpc

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRequestID(t *testing.T) {
	if !StringID("").IsZero() {
		// 空字符串 id 非 zero？StringID("") 会 marshal 成 `""`
	}
	zero := RequestID{}
	if !zero.IsZero() || zero.String() != "" {
		t.Fatal("zero id")
	}
	s := StringID("abc")
	if s.IsZero() || s.String() != "abc" {
		t.Fatalf("string id = %q", s.String())
	}
	n := NumberID(42)
	if n.IsZero() || n.String() != "42" {
		t.Fatalf("num id = %q", n.String())
	}

	// MarshalJSON
	bs, err := s.MarshalJSON()
	if err != nil || !strings.Contains(string(bs), "abc") {
		t.Fatalf("marshal = %q %v", bs, err)
	}
	bs, _ = zero.MarshalJSON()
	if string(bs) != "null" {
		t.Fatalf("zero marshal = %q", bs)
	}

	// UnmarshalJSON
	var id RequestID
	if err := id.UnmarshalJSON([]byte("null")); err != nil || !id.IsZero() {
		t.Fatalf("null = %v", err)
	}
	if err := id.UnmarshalJSON([]byte(`"x"`)); err != nil || id.String() != "x" {
		t.Fatalf("str = %q %v", id.String(), err)
	}
	if err := id.UnmarshalJSON([]byte(`7`)); err != nil || id.String() != "7" {
		t.Fatalf("num = %q %v", id.String(), err)
	}
	if err := id.UnmarshalJSON([]byte(`"  "`)); err == nil {
		t.Fatal("blank id")
	}
	if err := id.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("bad id")
	}
}

func TestNewMessages(t *testing.T) {
	req, err := NewRequest(StringID("1"), "m", nil)
	if err != nil || req.Method != "m" || req.ID.IsZero() {
		t.Fatalf("req = %+v %v", req, err)
	}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}

	ntf, err := NewNotification("n", map[string]any{"a": 1})
	if err != nil || ntf.Method != "n" {
		t.Fatalf("ntf = %+v %v", ntf, err)
	}
	if err := ntf.Validate(); err != nil {
		t.Fatal(err)
	}

	res, err := NewResultResponse(StringID("1"), "ok")
	if err != nil || res.Error != nil {
		t.Fatalf("result = %+v %v", res, err)
	}
	if err := res.Validate(); err != nil {
		t.Fatal(err)
	}

	er, err := NewErrorResponse(StringID("1"), -1, "e", nil)
	if err != nil || er.Error == nil {
		t.Fatalf("error resp = %+v %v", er, err)
	}
	if err := er.Validate(); err != nil {
		t.Fatal(err)
	}

	// 零 ID 校验失败
	if err := (Request{}).Validate(); err == nil {
		t.Fatal("zero id")
	}
	if err := (Request{ID: StringID("1")}).Validate(); err == nil {
		t.Fatal("empty method")
	}
}

func TestNewRPCError(t *testing.T) {
	if NewRPCError(nil) != nil {
		t.Fatal("nil")
	}
	e := NewRPCError(&Error{Code: 1, Message: "m", Data: json.RawMessage(`{"method":"x","reason":"r"}`)})
	if e.Method != "x" || e.Reason != "r" {
		t.Fatalf("rpc = %+v", e)
	}
	if e.Error() == "" {
		t.Fatal("Error()")
	}
	var nilE *RPCError
	if nilE.Error() != "" {
		t.Fatal("nil error")
	}
	if NewRPCError(&Error{Code: 1, Message: " "}).Error() == "" {
		t.Fatal("blank msg")
	}
	if NewRPCError(&Error{Code: 1}).Error() == "" {
		t.Fatal("no msg")
	}
	_ = errors.New("x")
}
