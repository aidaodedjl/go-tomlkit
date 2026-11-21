package tomlkit

import "testing"

// I3：invalid UTF-8 的 string Set 必须报错
func TestFixI3InvalidUTF8(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	if err := doc.Set("s", "a\xffb"); err == nil {
		t.Fatal("invalid UTF-8 应报错")
	}
}

// I4：Array 与 InlineTable 的 API 完整性
func TestFixI4ArrayInlineAPI(t *testing.T) {
	doc := mustParseOk(t, "arr = [1, 2]\nit = {x = 1}\n")
	arr, _ := doc.Get("arr").Array()
	if arr.Len() != 2 {
		t.Fatal("Array.Len")
	}
	if v, ok := arr.Get(1).AsInteger(); !ok || v != 2 {
		t.Fatal("Array.Get")
	}
	if arr.Get(5) != nil {
		t.Fatal("Array.Get 越界应 nil")
	}
	if err := arr.Append("three"); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("Append 后不合法：%v\n%s", perr, out)
	}
	re := mustParseOk(t, out)
	reArr, _ := re.Get("arr").Array()
	if reArr.Len() != 3 {
		t.Fatalf("Append 语义：%q", out)
	}
	itb, _ := doc.Get("it").InlineTable()
	if len(itb.Keys()) != 1 || itb.Keys()[0] != "x" {
		t.Fatal("InlineTable.Keys")
	}
	if itb.Get("x") == nil || itb.Get("zz") != nil {
		t.Fatal("InlineTable.Get")
	}
	if !itb.Delete("x") || itb.Get("x") != nil {
		t.Fatal("InlineTable.Delete")
	}
	if _, perr := parse([]byte(doc.String())); perr != nil {
		t.Fatal("Delete 后不合法")
	}
}

// M3：复合值内部的控制字符应拒绝
func TestFixM3ControlInCompound(t *testing.T) {
	if _, perr := parse([]byte("a = [1,\r2]\n")); perr == nil {
		t.Fatal("数组内裸 CR 应拒绝")
	}
	if _, perr := parse([]byte("a = [1, 2] # \x0b\n")); perr == nil {
		t.Fatal("注释控制字符在 lexer 层已拦，此断言防御复合内注释路径")
	}
}

// M4：值类解析错误应有 Snippet
func TestFixM4ValueErrorSnippet(t *testing.T) {
	_, err := Parse([]byte("a = 0x\n"))
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("应返回 *ParseError：%T", err)
	}
	if pe.Snippet == "" {
		t.Fatal("Snippet 不应为空")
	}
	if pe.Line != 1 {
		t.Fatalf("行号：%d", pe.Line)
	}
}
