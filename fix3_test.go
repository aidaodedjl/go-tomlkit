package tomlkit

import (
	"strings"
	"testing"
)

// C4：整表替换内容行数超过原区块时不得溢出到兄弟表
func TestFixC4ReplaceOverflow(t *testing.T) {
	doc := mustParseOk(t, "[a]\nx = 1\n[b]\ny = 2\n")
	nt := NewTable()
	for i := 1; i <= 5; i++ {
		_ = nt.Set("p"+strings.Repeat("q", i), int64(i))
	}
	if err := doc.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("C4 不合法：%v\n%s", perr, out)
	}
	re := mustParseOk(t, out)
	at, _ := re.Get("a").Table()
	if len(at.Keys()) != 5 {
		t.Fatalf("C4 键归属漂移：a=%v\n%s", at.Keys(), out)
	}
	bt, _ := re.Get("b").Table()
	if len(bt.Keys()) != 1 || bt.Keys()[0] != "y" {
		t.Fatalf("C4 b 被污染：%v\n%s", bt.Keys(), out)
	}
	if !strings.Contains(out, "[b]\ny = 2\n") {
		t.Fatalf("C4 b 区块位置：%q", out)
	}
}

// C5：构造表内容里 kv 与子表头的顺序（kv 必须先于子表头）
func TestFixC5BuilderOrder(t *testing.T) {
	nt := NewTable()
	sub := NewTable()
	_ = sub.Set("s", int64(1))
	_ = nt.Set("sub", sub)
	_ = nt.Set("plain", int64(2))
	doc := NewDocument()
	if err := doc.Set("new", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	re := mustParseOk(t, out)
	if v, ok := re.GetPath("new.plain").AsInteger(); !ok || v != 2 {
		t.Fatalf("C5 plain 归属错位：%q", out)
	}
}

// C6：嵌套表上 Set 子表：表头路径与父拼接，区块落在父内
func TestFixC6NestedSubtable(t *testing.T) {
	doc := mustParseOk(t, "q = 0\n[a]\nx = 1\n")
	at, _ := doc.Get("a").Table()
	sub := NewTable()
	_ = sub.Set("s", int64(9))
	if err := at.Set("sub", sub); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.Contains(out, "[a.sub]\ns = 9\n") {
		t.Fatalf("C6 表头路径：%q", out)
	}
	re := mustParseOk(t, out)
	if v, ok := re.GetPath("a.sub.s").AsInteger(); !ok || v != 9 {
		t.Fatalf("C6 语义：%q", out)
	}
}

// C8：同一点连续追加不得耗尽 seq 中点（root 与嵌套表）
func TestFixC8SeqExhaustion(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n[t]\nx = 1\n")
	for i := 0; i < 80; i++ {
		if err := doc.Set("k"+strings.Repeat("z", i+1), int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatalf("C8 不合法：%v", perr)
	}
	if len(re.Keys()) != 82 { // a + 80 新键 + t
		t.Fatalf("C8 root 键数：%d\n%s", len(re.Keys()), out)
	}
	// 嵌套表上同样验证：[t] 带子表
	doc2 := mustParseOk(t, "r = 0\n[t]\nx = 1\n[t.sub]\ny = 2\n")
	tb, _ := doc2.Get("t").Table()
	for i := 0; i < 80; i++ {
		if err := tb.Set("k"+strings.Repeat("w", i+1), int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	out2 := doc2.String()
	re2, perr2 := parse([]byte(out2))
	if perr2 != nil {
		t.Fatalf("C8 嵌套不合法：%v\n%s", perr2, out2)
	}
	rt, _ := re2.Get("t").Table()
	if len(rt.Keys()) != 82 { // x + 80 新键 + sub
		t.Fatalf("C8 嵌套键归属：%d\n%s", len(rt.Keys()), out2)
	}
	// 键集合精确断言（不只长度）
	if rt.Keys()[0] != "x" || rt.Keys()[81] != "sub" {
		t.Fatalf("C8 嵌套键序：%v…", rt.Keys()[:3])
	}
	if v, ok := re2.GetPath("r").AsInteger(); !ok || v != 0 {
		t.Fatalf("C8 root 键归属：%q", out2)
	}
}

// C9：无表头表（dotted/隐式中间表）上直接 Set 新键：keyRaw 带路径前缀
func TestFixC9SetOnImplicitTable(t *testing.T) {
	// a) dotted 中间表
	doc := mustParseOk(t, "a.b = 1\n")
	at, _ := doc.Get("a").Table()
	if err := at.Set("c", int64(3)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	re := mustParseOk(t, out)
	if v, ok := re.GetPath("a.c").AsInteger(); !ok || v != 3 {
		t.Fatalf("C9a 语义错位：%q", out)
	}
	// b) [a.b] 的隐式中间表
	doc2 := mustParseOk(t, "[a.b]\nz = 1\n")
	at2, _ := doc2.Get("a").Table()
	if err := at2.Set("x", int64(1)); err != nil {
		t.Fatal(err)
	}
	out2 := doc2.String()
	re2 := mustParseOk(t, out2)
	if v, ok := re2.GetPath("a.x").AsInteger(); !ok || v != 1 {
		t.Fatalf("C9b 语义错位：%q", out2)
	}
}
