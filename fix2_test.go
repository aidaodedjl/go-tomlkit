package tomlkit

import "testing"

// C1：内联表成员修改必须反映到写回
func TestFixC1InlineMemberEdit(t *testing.T) {
	doc := mustParseOk(t, "a = {x = 1}\n")
	it, _ := doc.Get("a").InlineTable()
	if err := it.Set("x", int64(2)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc.String(), "a = {x = 2}\n"; got != want {
		t.Fatalf("C1：%q want %q", got, want)
	}
}

// C2：内联表内 dotted 成员重渲染合法
func TestFixC2InlineDottedMember(t *testing.T) {
	doc := mustParseOk(t, "x = {a.b = 1}\n")
	it, _ := doc.Get("x").InlineTable()
	if err := it.Set("c", int64(2)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("C2 写回不合法：%v\n%s", perr, out)
	}
	re := mustParseOk(t, out)
	if v, ok := re.GetPath("x.a.b").AsInteger(); !ok || v != 1 {
		t.Fatalf("C2 dotted 语义：%q", out)
	}
	if v, ok := re.GetPath("x.c").AsInteger(); !ok || v != 2 {
		t.Fatalf("C2 新成员语义：%q", out)
	}
}
