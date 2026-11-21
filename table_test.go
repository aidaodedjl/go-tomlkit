package tomlkit

import "testing"

func TestGetNavigation(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n[t]\nb = 2\n[t.sub]\nc = 3\n")
	if doc.Get("a") == nil {
		t.Fatal("根 Get")
	}
	if v, ok := doc.Get("a").AsInteger(); !ok || v != 1 {
		t.Fatal("Get a")
	}
	tt := doc.Get("t")
	if tt == nil {
		t.Fatal("Get t")
	}
	ttb, ok := tt.Table()
	if !ok {
		t.Fatal("t 不是表")
	}
	if sub := ttb.Get("sub"); sub == nil {
		t.Fatal("嵌套 Get")
	}
	if doc.Get("missing") != nil {
		t.Fatal("未命中应返回 nil")
	}
	if ttb.Get("missing") != nil {
		t.Fatal("表内未命中应返回 nil")
	}
}

func TestKeysOrder(t *testing.T) {
	doc := mustParseOk(t, "z = 1\na = 2\nm = 3\n")
	if k := doc.Keys(); len(k) != 3 || k[0] != "z" || k[1] != "a" || k[2] != "m" {
		t.Fatalf("出现序不符：%v", k)
	}
	doc = mustParseOk(t, "[b]\nx = 1\n[a]\ny = 2\n")
	if k := doc.Keys(); len(k) != 2 || k[0] != "b" || k[1] != "a" {
		t.Fatalf("表序不符：%v", k)
	}
	bb, _ := doc.Get("b").Table()
	if k := bb.Keys(); len(k) != 1 || k[0] != "x" {
		t.Fatalf("表内键序不符：%v", k)
	}
}

func TestDeleteScalar(t *testing.T) {
	src := "keep = 1\n\n# c 注释\ndrop = 2\n\nkeep2 = 3\n"
	doc := mustParseOk(t, src)
	if !doc.Delete("drop") {
		t.Fatal("Delete 应返回 true")
	}
	want := "keep = 1\n\nkeep2 = 3\n"
	if got := doc.String(); got != want {
		t.Fatalf("删除后输出不符：\n%q\nwant %q", got, want)
	}
	if doc.Delete("drop") {
		t.Fatal("再删应返回 false")
	}
}

func TestDeleteTableBlock(t *testing.T) {
	src := "a = 1\n\n[t]\nx = 1\n[t.sub]\ny = 2\n\n[u]\nz = 3\n"
	doc := mustParseOk(t, src)
	if !doc.Delete("t") {
		t.Fatal("Delete t")
	}
	want := "a = 1\n\n[u]\nz = 3\n"
	if got := doc.String(); got != want {
		t.Fatalf("整表删除不符：\n%q\nwant %q", got, want)
	}
}

func TestDeleteInTable(t *testing.T) {
	src := "[t]\nkeep = 1\ndrop = 2\n"
	doc := mustParseOk(t, src)
	tb, _ := doc.Get("t").Table()
	if !tb.Delete("drop") {
		t.Fatal("表内 Delete")
	}
	if got, want := doc.String(), "[t]\nkeep = 1\n"; got != want {
		t.Fatalf("表内删除不符：%q want %q", got, want)
	}
}

func TestDeleteAoT(t *testing.T) {
	src := "[[a]]\nx = 1\n[[a]]\nx = 2\n[b]\ny = 1\n"
	doc := mustParseOk(t, src)
	if !doc.Delete("a") {
		t.Fatal("Delete AoT")
	}
	if got, want := doc.String(), "[b]\ny = 1\n"; got != want {
		t.Fatalf("AoT 删除不符：%q want %q", got, want)
	}
}
