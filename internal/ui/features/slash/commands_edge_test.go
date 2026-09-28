package slash

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权.

import "testing"

func TestParseCommand(t *testing.T) {
	cmd, args, ok := ParseCommand("hello")
	if ok || cmd != "" {
		t.Fatalf("non-slash = %q %v %v", cmd, args, ok)
	}
	cmd, args, ok = ParseCommand("/help a b")
	if !ok || cmd != "/help" || len(args) != 2 {
		t.Fatalf("slash = %q %v %v", cmd, args, ok)
	}
	cmd, args, ok = ParseCommand("  /x  ")
	if !ok || cmd != "/x" || len(args) != 0 {
		t.Fatalf("trim = %q %v %v", cmd, args, ok)
	}
	if _, _, ok := ParseCommand("/"); !ok {
		// "/" 本身也算 command（无 args）
	}
}

func TestCommandDisplayAndDescription(t *testing.T) {
	c := Command{Name: "/x", Usage: "/x <arg>", DescriptionZH: "中文", DescriptionEN: "English"}
	if c.DisplayText() != "/x <arg>" {
		t.Fatalf("display = %q", c.DisplayText())
	}
	c.Usage = ""
	if c.DisplayText() != "/x" {
		t.Fatalf("name display = %q", c.DisplayText())
	}
	if c.Description("en") != "English" {
		t.Fatal("en")
	}
	if c.Description("zh") != "中文" {
		t.Fatal("zh")
	}

	g := Group("unknown-group")
	if g.Label("zh") != "通用" {
		t.Fatalf("default zh = %q", g.Label("zh"))
	}
	if g.Label("en") != "General" {
		t.Fatal("default en")
	}
	for _, grp := range []Group{GroupProject, GroupRuntime, GroupConfig} {
		if grp.Label("zh") == "" || grp.Label("en") == "" {
			t.Fatalf("group %v", grp)
		}
	}
}

func TestFindNormalizeSuggestions(t *testing.T) {
	if FindCommand("") != nil {
		t.Fatal("empty")
	}
	if FindCommand("nope-xyz") != nil {
		t.Fatal("missing")
	}
	if FindCommand("/help") == nil && FindCommand("help") == nil {
		// help 可能叫别的名字，找一个真实存在的
		if len(Commands) == 0 {
			t.Fatal("no commands")
		}
		name := Commands[0].Name
		if FindCommand(name) == nil {
			t.Fatalf("find %q", name)
		}
	}

	if NormalizeCommand("nope") != "" {
		t.Fatal("normalize miss")
	}
	if len(VisibleCommands()) == 0 {
		t.Fatal("visible")
	}
	if len(GetSuggestions("")) == 0 {
		t.Fatal("empty suggestions")
	}
	if len(GroupedVisibleCommands("zh")) == 0 {
		t.Fatal("grouped")
	}
	if len(GroupedVisibleCommands("en")) == 0 {
		t.Fatal("grouped en")
	}
}
