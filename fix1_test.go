package tomlkit

import "testing"

// C7：无尾换行文件追加 kv，旧末行必须补换行（写回合法性优先，
// 对未触及内容的最小必要修改）
func TestFixC7NoTrailingNewlineAppend(t *testing.T) {
	doc := mustParseOk(t, "a = 1")
	if err := doc.Set("b", int64(2)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc.String(), "a = 1\nb = 2\n"; got != want {
		t.Fatalf("C7：%q want %q", got, want)
	}
	// 表内同样
	doc2 := mustParseOk(t, "[t]\nx = 1")
	tb, _ := doc2.Get("t").Table()
	if err := tb.Set("y", int64(2)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc2.String(), "[t]\nx = 1\ny = 2\n"; got != want {
		t.Fatalf("C7 表内：%q want %q", got, want)
	}
}

// M1：CRLF 文件末条目为 AoT 时，追加 eol 跟随 CRLF
func TestFixM1AoTTrailingEOL(t *testing.T) {
	doc := mustParseOk(t, "[[t]]\r\nx = 1\r\n")
	if err := doc.Set("b", int64(2)); err != nil {
		t.Fatal(err)
	}
	if got, want := doc.String(), "b = 2\r\n[[t]]\r\nx = 1\r\n"; got != want {
		t.Fatalf("M1：%q want %q", got, want)
	}
}

// C3：标量替换复合条目（表/dotted 表/AoT）输出必须合法且语义正确
func TestFixC3ScalarReplacesComposite(t *testing.T) {
	// a) 替换 [a] 表
	doc := mustParseOk(t, "q = 1\n\n[a]\nx = 1\n")
	if err := doc.Set("a", int64(5)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("C3a 写回不合法：%v\n%s", perr, out)
	}
	if v, ok := mustParseOk(t, out).Get("a").AsInteger(); !ok || v != 5 {
		t.Fatalf("C3a 语义：%q", out)
	}
	// b) 替换 dotted 中间表
	doc = mustParseOk(t, "x = 0\na.b = 1\n")
	if err := doc.Set("a", int64(5)); err != nil {
		t.Fatal(err)
	}
	out = doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("C3b 写回不合法：%v\n%s", perr, out)
	}
	if v, ok := mustParseOk(t, out).Get("a").AsInteger(); !ok || v != 5 {
		t.Fatalf("C3b 语义：%q", out)
	}
	// c) 替换 AoT
	doc = mustParseOk(t, "[[a]]\nx = 1\n")
	if err := doc.Set("a", int64(5)); err != nil {
		t.Fatal(err)
	}
	out = doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("C3c 写回不合法：%v\n%s", perr, out)
	}
	if v, ok := mustParseOk(t, out).Get("a").AsInteger(); !ok || v != 5 {
		t.Fatalf("C3c 语义：%q", out)
	}
}
