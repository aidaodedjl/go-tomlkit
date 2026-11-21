package tomlkit

import (
	"strings"
	"testing"
)

// B8：AoT 的 Set/Append 语义
func TestFixB8AoTEdit(t *testing.T) {
	// Append(NewTable()) 应追加一个 [[x]] 元素
	doc := mustParseOk(t, "[[x]]\na = 1\n")
	arr, _ := doc.Get("x").Array()
	nt := NewTable()
	_ = nt.Set("b", int64(2))
	if err := arr.Append(nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("B8 Append 写回不合法：%v\n%s", perr, out)
	}
	re := mustParseOk(t, out)
	rarr, _ := re.Get("x").Array()
	if rarr.Len() != 2 {
		t.Fatalf("B8 Append 语义：%q", out)
	}
	// Set(0, NewTable) 替换首元素
	doc2 := mustParseOk(t, "[[x]]\na = 1\n")
	arr2, _ := doc2.Get("x").Array()
	nt2 := NewTable()
	_ = nt2.Set("z", true)
	if err := arr2.Set(0, nt2); err != nil {
		t.Fatal(err)
	}
	out2 := doc2.String()
	if _, perr := parse([]byte(out2)); perr != nil {
		t.Fatalf("B8 Set 写回不合法：%v\n%s", perr, out2)
	}
	if v, ok := mustParseOk(t, out2).GetPath("x[0].z").(Item); ok || v != nil {
		_ = v // 路径糖不穿 AoT（M29），直接查树
	}
	rarr2, _ := mustParseOk(t, out2).Get("x").Array()
	el, _ := rarr2.Get(0).Table()
	if v, ok := el.Get("z").AsBool(); !ok || !v {
		t.Fatalf("B8 Set 语义：%q", out2)
	}
	// 标量塞进 AoT 应报错
	if err := arr2.Set(0, 5); err == nil {
		t.Fatal("标量替换 AoT 元素应报错")
	}
}

// B9：空表 EOF 无换行，追加键合法
func TestFixB9EmptyTableNoEOL(t *testing.T) {
	doc := mustParseOk(t, "[a]")
	tb, _ := doc.Get("a").Table()
	if err := tb.Set("k", int64(1)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("B9 写回不合法：%v\n%q", perr, out)
	}
	if !strings.Contains(out, "[a]\nk = 1") {
		t.Fatalf("B9 输出：%q", out)
	}
}

// M10：引号表头键含 ]
func TestFixM10QuotedHeaderBracket(t *testing.T) {
	doc := mustParseOk(t, "[\"a]b\"]\nx = 1\n")
	tb, _ := doc.Get("a]b").Table()
	if tb == nil {
		t.Fatal("M10 引号表头")
	}
	if got := doc.String(); got != "[\"a]b\"]\nx = 1\n" {
		t.Fatalf("M10 保真：%q", got)
	}
}

// M11：表头尾点拒绝
func TestFixM11HeaderTrailingDot(t *testing.T) {
	for _, bad := range []string{"[a.]\nx = 1\n", "[[a.]]\nx = 1\n", "[a. ]\n", "[\"a\".]\n"} {
		if _, perr := parse([]byte(bad)); perr == nil {
			t.Fatalf("M11 应拒绝：%q", bad)
		}
	}
}

// M12：非法 UTF-8 键报错
func TestFixM12KeyUTF8(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	if err := doc.Set("k\xff", int64(1)); err == nil {
		t.Fatal("非法 UTF-8 键应报错")
	}
	if err := doc.SetPath("k\xff.sub", int64(1)); err == nil {
		t.Fatal("SetPath 非法键应报错")
	}
}

// M13：DeletePath 穿内联表
func TestFixM13DeletePathInline(t *testing.T) {
	doc := mustParseOk(t, "a = {b = 1, c = 2}\n")
	if !doc.DeletePath("a.b") {
		t.Fatal("M13 DeletePath")
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("M13 写回不合法：%v", perr)
	}
	if v := mustParseOk(t, out).GetPath("a.c"); v == nil {
		t.Fatalf("M13 语义：%q", out)
	}
}

// M14：主导换行风格（CRLF 多数文件追加用 CRLF）
func TestFixM14DominantEOL(t *testing.T) {
	doc := mustParseOk(t, "a = 1\r\nb = 2\r\nc = 3\n")
	if err := doc.Set("d", int64(4)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.Contains(out, "d = 4\r\n") {
		t.Fatalf("M14 追加应 CRLF：%q", out)
	}
	// 新表前置空行也是主导风格
	doc2 := mustParseOk(t, "a = 1\r\n")
	nt := NewTable()
	_ = nt.Set("k", int64(1))
	_ = doc2.Set("t", nt)
	if !strings.Contains(doc2.String(), "\r\n\r\n[t]\r\n") {
		t.Fatalf("M14 新表空行：%q", doc2.String())
	}
}

// M15：非法 UTF-8 错误行号
func TestFixM15UTF8ErrorLine(t *testing.T) {
	_, perr := parse([]byte("a = 1\nb = 2\nc = \xff\n"))
	if perr == nil {
		t.Fatal("应报错")
	}
	if perr.Line != 3 {
		t.Fatalf("M15 行号：%d（want 3）", perr.Line)
	}
}

// M16：引号键内原始控制字符拒绝
func TestFixM16QuotedKeyControl(t *testing.T) {
	if _, perr := parse([]byte("\"\x01\" = 1\n")); perr == nil {
		t.Fatal("M16 引号键控制字符应拒绝")
	}
}
