package tomlkit

import (
	"strings"
	"testing"
)

func TestGetPath(t *testing.T) {
	doc := mustParseOk(t, "a = 1\nd.k = 4\n[t]\nx = 2\n[t.sub]\ny = 3\n")
	if v, ok := doc.GetPath("t.sub.y").AsInteger(); !ok || v != 3 {
		t.Fatal("GetPath 深层")
	}
	if v, ok := doc.GetPath("a").AsInteger(); !ok || v != 1 {
		t.Fatal("GetPath 标量")
	}
	if doc.GetPath("t.missing") != nil {
		t.Fatal("未命中应 nil")
	}
	if doc.GetPath("a.deeper") != nil {
		t.Fatal("标量下导航应 nil")
	}
	if v, ok := doc.GetPath("d.k").AsInteger(); !ok || v != 4 {
		t.Fatal("GetPath dotted")
	}
}

func TestSetPathCreatesDotted(t *testing.T) {
	doc := mustParseOk(t, "top = 1\n")
	if err := doc.SetPath("a.b.c", int64(1)); err != nil {
		t.Fatal(err)
	}
	want := "top = 1\na.b.c = 1\n"
	if got := doc.String(); got != want {
		t.Fatalf("dotted 生成不符：%q want %q", got, want)
	}
	// 重解析语义一致
	re, perr := parse([]byte(doc.String()))
	if perr != nil {
		t.Fatal(perr)
	}
	if v, ok := re.GetPath("a.b.c").AsInteger(); !ok || v != 1 {
		t.Fatal("语义校验")
	}
}

func TestSetPathHitsExisting(t *testing.T) {
	doc := mustParseOk(t, "a.b = 1\nc = 2\n")
	if err := doc.SetPath("a.b", int64(9)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc.String(), "a.b = 9\nc = 2\n"; got != want {
		t.Fatalf("dotted 命中替换不符：%q want %q", got, want)
	}
	// 普通表内键
	doc2 := mustParseOk(t, "[t]\nx = 1\n")
	if err := doc2.SetPath("t.x", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc2.String(), "x = true") {
		t.Fatalf("表内路径替换不符：%q", doc2.String())
	}
}

func TestSetPathConflict(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	if err := doc.SetPath("a.b", int64(1)); err == nil {
		t.Fatal("标量下钻应报错")
	}
}

func TestDeletePath(t *testing.T) {
	doc := mustParseOk(t, "a.b = 1\nkeep = 2\n")
	if !doc.DeletePath("a.b") {
		t.Fatal("DeletePath")
	}
	if got, want := doc.String(), "keep = 2\n"; got != want {
		t.Fatalf("dotted 删除不符：%q want %q", got, want)
	}
	if doc.DeletePath("a.b") {
		t.Fatal("再删 false")
	}
}

func TestPathKeysVariants(t *testing.T) {
	doc := mustParseOk(t, "\"a.b\" = 1\nplain = 2\n")
	if v, ok := doc.GetPathKeys([]string{"a.b"}).AsInteger(); !ok || v != 1 {
		t.Fatal("含点引号键")
	}
	if err := doc.SetPathKeys([]string{"x.y", "z"}, int64(3)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `"x.y".z = 3`) {
		t.Fatalf("Keys 变体 dotted 生成不符：%q", doc.String())
	}
	if !doc.DeletePathKeys([]string{"a.b"}) {
		t.Fatal("DeletePathKeys")
	}
}

func TestPathSugarConsistency(t *testing.T) {
	// SetPath 与逐级 Get+Set 语义一致
	d1 := mustParseOk(t, "[t]\nx = 1\n")
	d2 := mustParseOk(t, "[t]\nx = 1\n")
	_ = d1.SetPath("t.x", int64(5))
	tb, _ := d2.Get("t").Table()
	_ = tb.Set("x", int64(5))
	if d1.String() != d2.String() {
		t.Fatalf("语义不一致：\n%q\n%q", d1.String(), d2.String())
	}
	// 新键同样一致（都追加表尾）
	d3 := mustParseOk(t, "[t]\nx = 1\n")
	d4 := mustParseOk(t, "[t]\nx = 1\n")
	_ = d3.SetPath("t.new", "v")
	tb4, _ := d4.Get("t").Table()
	_ = tb4.Set("new", "v")
	if d3.String() != d4.String() {
		t.Fatalf("新键不一致：\n%q\n%q", d3.String(), d4.String())
	}
}
