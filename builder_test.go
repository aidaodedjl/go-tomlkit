package tomlkit

import (
	"strings"
	"testing"
)

func TestBuilderEmptyDocument(t *testing.T) {
	doc := NewDocument()
	if got := doc.String(); got != "" {
		t.Fatalf("空文档应输出空串：%q", got)
	}
}

func TestBuilderFromScratch(t *testing.T) {
	// TestMotion toml_handler.py 无原文分支：play_audio + actions 表
	doc := NewDocument()
	_ = doc.Set("play_audio", true)
	actions := NewTable()
	wave := NewTable()
	_ = wave.Set("name", "wave")
	_ = wave.Set("volume", 0.8)
	_ = actions.Set("wave", wave)
	if err := doc.Set("actions", actions); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	want := "play_audio = true\n\n[actions]\n\n[actions.wave]\nname = \"wave\"\nvolume = 0.8\n"
	if out != want {
		t.Fatalf("从零构建不符：\n%q\nwant %q", out, want)
	}
	// 再解析语义等价
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatal(perr)
	}
	if v, ok := re.GetPath("actions.wave.volume").AsFloat(); !ok || v != 0.8 {
		t.Fatal("语义校验")
	}
}

func TestBuilderNewInlineTableAndArray(t *testing.T) {
	doc := NewDocument()
	it := NewInlineTable()
	_ = it.Set("a", int64(1))
	arr := NewArray()
	_ = doc.Set("it", it)
	if err := doc.Set("arr", arr); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.Contains(out, "it = {a = 1}") {
		t.Fatalf("内联表构建不符：%q", out)
	}
	if !strings.Contains(out, "arr = []") {
		t.Fatalf("空数组构建不符：%q", out)
	}
}

func TestBuilderNestedTables(t *testing.T) {
	doc := NewDocument()
	deep := NewTable()
	deeper := NewTable()
	_ = deeper.Set("leaf", int64(1))
	_ = deep.Set("sub", deeper)
	_ = doc.Set("deep", deep)
	out := doc.String()
	if !strings.Contains(out, "[deep.sub]\nleaf = 1\n") {
		t.Fatalf("嵌套构建不符：%q", out)
	}
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("写回不合法：%v", perr)
	}
}
