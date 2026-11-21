package tomlkit

import (
	"strings"
	"testing"
)

func TestSetTableReplaceKeepsHeader(t *testing.T) {
	src := "top = 1\n\n# actions 表注释\n[actions]\nold = \"x\"\n\n[actions.sub]\ndeep = true\n\n[after]\nk = 1\n"
	doc := mustParseOk(t, src)
	nt := NewTable()
	_ = nt.Set("fresh", int64(9))
	_ = nt.Set("another", "v")
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	// 表头原位（含前导注释与空行），旧区块内容消失，新键在区块内，后续表不动
	wantSub := "top = 1\n\n# actions 表注释\n[actions]\nfresh = 9\nanother = \"v\"\n\n[after]\nk = 1\n"
	if out != wantSub {
		t.Fatalf("整表替换不符：\n%q\nwant %q", out, wantSub)
	}
}

func TestSetTableAppendAtEnd(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n[t]\nx = 1\n")
	nt := NewTable()
	_ = nt.Set("y", int64(2))
	if err := doc.Set("new", nt); err != nil {
		t.Fatal(err)
	}
	want := "a = 1\n[t]\nx = 1\n\n[new]\ny = 2\n"
	if got := doc.String(); got != want {
		t.Fatalf("新表追加不符：\n%q\nwant %q", got, want)
	}
}

func TestSetTableReplaceEmptyRegion(t *testing.T) {
	doc := mustParseOk(t, "[actions]\n\n# 注释也换掉\nold = 1\n")
	nt := NewTable()
	_ = nt.Set("n", true)
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	want := "[actions]\nn = true\n"
	if got := doc.String(); got != want {
		t.Fatalf("空区块替换不符：%q want %q", got, want)
	}
}

func TestSetTableNestedNewContent(t *testing.T) {
	doc := mustParseOk(t, "[actions]\nold = 1\n")
	nt := NewTable()
	sub := NewTable()
	_ = sub.Set("deep", int64(1))
	_ = nt.Set("wave", sub)
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.Contains(out, "[actions]\n\n[actions.wave]\ndeep = 1\n") {
		t.Fatalf("嵌套新内容不符：%q", out)
	}
	// 重解析语义一致
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatal(perr)
	}
	tb, _ := re.Get("actions").Table()
	wave, _ := tb.Get("wave").Table()
	if v, ok := wave.Get("deep").AsInteger(); !ok || v != 1 {
		t.Fatal("语义校验失败")
	}
}

func TestSetTableReplacesAoT(t *testing.T) {
	doc := mustParseOk(t, "[[a]]\nx = 1\n[[a]]\nx = 2\n[b]\ny = 1\n")
	nt := NewTable()
	_ = nt.Set("z", int64(3))
	if err := doc.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	want := "[a]\nz = 3\n[b]\ny = 1\n"
	if got := doc.String(); got != want {
		t.Fatalf("AoT 整表替换不符：%q want %q", got, want)
	}
}

// TestMotion toml_handler.py 场景等价：del doc["actions"] + 重新赋表
func TestSetTableTestMotionScenario(t *testing.T) {
	src := "play_audio = false\n\n[actions.wave]\nname = \"wave\"\nvolume = 0.8\n"
	doc := mustParseOk(t, src)
	nt := NewTable()
	wave := NewTable()
	_ = wave.Set("name", "wave2")
	_ = wave.Set("volume", 1.0)
	_ = nt.Set("wave", wave)
	_ = doc.Set("play_audio", true)
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.HasPrefix(out, "play_audio = true\n") {
		t.Fatalf("标量替换不符：%q", out)
	}
	if !strings.Contains(out, "[actions.wave]\nname = \"wave2\"\nvolume = 1.0\n") {
		t.Fatalf("actions 区块不符：%q", out)
	}
}
