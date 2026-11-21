package tomlkit

import (
	"testing"
)

var bom = []byte{0xEF, 0xBB, 0xBF}

type wantTok struct {
	kind tokenKind
	raw  string
}

func assertTokens(t *testing.T, input []byte, want []wantTok) *ParseError {
	t.Helper()
	toks, perr := lex(input)
	if perr != nil {
		return perr
	}
	got := make([]wantTok, 0, len(toks))
	for _, tk := range toks {
		got = append(got, wantTok{tk.kind, tk.raw})
	}
	if len(got) != len(want) {
		t.Fatalf("token 数不符：got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] 不符：got %+v want %+v", i, got[i], want[i])
		}
	}
	return nil
}

func TestLexKV(t *testing.T) {
	if err := assertTokens(t, []byte("a = 1\n"), []wantTok{
		{tKey, "a"}, {tWhitespace, " "}, {tEquals, "="},
		{tWhitespace, " "}, {tValue, "1"}, {tNewline, "\n"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLexKVNoSpaces(t *testing.T) {
	if err := assertTokens(t, []byte("a=true"), []wantTok{
		{tKey, "a"}, {tEquals, "="}, {tValue, "true"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLexQuotedKeys(t *testing.T) {
	if err := assertTokens(t, []byte("\"a.b\" = 1\n'lit\\?' = 2\n"), []wantTok{
		{tKey, "\"a.b\""}, {tWhitespace, " "}, {tEquals, "="}, {tWhitespace, " "},
		{tValue, "1"}, {tNewline, "\n"},
		{tKey, "'lit\\?'"}, {tWhitespace, " "}, {tEquals, "="}, {tWhitespace, " "},
		{tValue, "2"}, {tNewline, "\n"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
	// 解码后的键名
	toks, err := lex([]byte("\"a.b\" = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(toks[0].parts) != 1 || toks[0].parts[0].name != "a.b" {
		t.Fatalf("引号键解码不符：got %+v", toks[0].parts)
	}
	toks, _ = lex([]byte("'lit\\?' = 2\n"))
	if toks[0].parts[0].name != "lit\\?" {
		t.Fatalf("字面键解码不符：got %+v", toks[0].parts)
	}
}

func TestLexTableHeader(t *testing.T) {
	toks, err := lex([]byte("[a.b]\n"))
	if err != nil {
		t.Fatal(err)
	}
	tk := toks[0]
	if tk.kind != tTableHeader || tk.raw != "[a.b]" || tk.aot {
		t.Fatalf("表头 token 不符： %+v", tk)
	}
	want := []keyPart{{name: "a", raw: "a"}, {name: "b", raw: "b"}}
	for i, p := range want {
		if tk.parts[i] != p {
			t.Fatalf("parts[%d] 不符：got %+v want %+v", i, tk.parts[i], p)
		}
	}
	if toks[1].kind != tNewline {
		t.Fatalf("表头后应为换行： got %+v", toks[1])
	}
}

func TestLexAoTHeader(t *testing.T) {
	toks, err := lex([]byte("[[fruit]]\n"))
	if err != nil {
		t.Fatal(err)
	}
	tk := toks[0]
	if tk.kind != tTableHeader || !tk.aot || tk.raw != "[[fruit]]" {
		t.Fatalf("AoT 表头不符： %+v", tk)
	}
	if len(tk.parts) != 1 || tk.parts[0].name != "fruit" {
		t.Fatalf("AoT parts 不符： %+v", tk.parts)
	}
}

func TestLexHeaderQuotedPart(t *testing.T) {
	toks, err := lex([]byte("[a.\"b.c\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	tk := toks[0]
	if len(tk.parts) != 2 || tk.parts[1].name != "b.c" || tk.parts[1].raw != "\"b.c\"" {
		t.Fatalf("引号段不符： %+v", tk.parts)
	}
}

func TestLexCRLF(t *testing.T) {
	if err := assertTokens(t, []byte("a = 1\r\nb = 2\r\n"), []wantTok{
		{tKey, "a"}, {tWhitespace, " "}, {tEquals, "="}, {tWhitespace, " "},
		{tValue, "1"}, {tNewline, "\r\n"},
		{tKey, "b"}, {tWhitespace, " "}, {tEquals, "="}, {tWhitespace, " "},
		{tValue, "2"}, {tNewline, "\r\n"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLexBareCRError(t *testing.T) {
	_, perr := lex([]byte("a = 1\rb = 2\n"))
	if perr == nil {
		t.Fatal("裸 CR 必须报错")
	}
	if perr.Line != 1 || perr.Snippet != "a = 1" {
		t.Fatalf("错误位置不符： %+v", perr)
	}
}

func TestLexBOMAtStart(t *testing.T) {
	input := append(append([]byte{}, bom...), []byte("a = 1\n")...)
	if err := assertTokens(t, input, []wantTok{
		{tBOM, string(bom)}, {tKey, "a"}, {tWhitespace, " "}, {tEquals, "="},
		{tWhitespace, " "}, {tValue, "1"}, {tNewline, "\n"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLexBOMNotAtStartError(t *testing.T) {
	input := append([]byte("a = 1\n"), bom...)
	input = append(input, []byte("b = 2\n")...)
	_, perr := lex(input)
	if perr == nil {
		t.Fatal("非开头 BOM 必须报错")
	}
	if perr.Line != 2 || perr.Col != 1 {
		t.Fatalf("错误位置不符： %+v", perr)
	}
}

func TestLexComment(t *testing.T) {
	if err := assertTokens(t, []byte("# hello\na = 1 # tail\n"), []wantTok{
		{tComment, "# hello"}, {tNewline, "\n"},
		{tKey, "a"}, {tWhitespace, " "}, {tEquals, "="}, {tWhitespace, " "},
		{tValue, "1"}, {tWhitespace, " "}, {tComment, "# tail"}, {tNewline, "\n"}, {tEOF, ""},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLexPositions(t *testing.T) {
	toks, err := lex([]byte("a = 1\nbb = 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	// 第二行的键：line 2, col 1, off 6（"a = 1\n" 共 6 字节）
	k := toks[6]
	if k.kind != tKey || k.line != 2 || k.col != 1 || k.off != 6 {
		t.Fatalf("第二行键位置不符： %+v", k)
	}
}

func TestLexEmpty(t *testing.T) {
	if err := assertTokens(t, []byte(""), []wantTok{{tEOF, ""}}); err != nil {
		t.Fatal(err)
	}
	if err := assertTokens(t, []byte("\n"), []wantTok{{tNewline, "\n"}, {tEOF, ""}}); err != nil {
		t.Fatal(err)
	}
}

// ---- Task 3: 值扫描器 ----

func assertValue(t *testing.T, input string, wantRaw string, wantKind valueKind) {
	t.Helper()
	toks, perr := lex([]byte(input))
	if perr != nil {
		t.Fatalf("lex 报错：%v", perr)
	}
	for _, tk := range toks {
		if tk.kind == tValue {
			if tk.raw != wantRaw {
				t.Fatalf("值 raw 不符：got %q want %q", tk.raw, wantRaw)
			}
			if tk.vkind != wantKind {
				t.Fatalf("值 kind 不符：raw %q got %v want %v", tk.raw, tk.vkind, wantKind)
			}
			return
		}
	}
	t.Fatalf("未找到值 token：%v", toks)
}

func TestLexValueStringEscapes(t *testing.T) {
	assertValue(t, `a = "b\tc\n\"\\"`+"\n", `"b\tc\n\"\\"`, vString)
	assertValue(t, `a = "\u00e9 \U0001F600"`+"\n", `"\u00e9 \U0001F600"`, vString)
	assertValue(t, `a = 'C:\path\no\escape'`+"\n", `'C:\path\no\escape'`, vString)
}

func TestLexValueStringHashAndBrackets(t *testing.T) {
	// 字符串内的 # ] } 不截断值、不当注释
	assertValue(t, "a = \"x # ] } y\"\n", "\"x # ] } y\"", vString)
	toks, perr := lex([]byte("a = \"x # y\"\n"))
	if perr != nil {
		t.Fatal(perr)
	}
	for _, tk := range toks {
		if tk.kind == tComment {
			t.Fatalf("字符串内的 # 不应是注释：%v", toks)
		}
	}
}

func TestLexValueUnterminatedString(t *testing.T) {
	_, perr := lex([]byte("a = \"abc\n"))
	if perr == nil || perr.Line != 1 {
		t.Fatalf("未闭合基本串必须报错且行号正确：%v", perr)
	}
	_, perr = lex([]byte("a = '''abc\n"))
	if perr == nil {
		t.Fatal("未闭合多行字面串必须报错")
	}
}

func TestLexValueMultilineBasic(t *testing.T) {
	input := "a = \"\"\"\nline1\nline2\\\n  continued\n\"\"\"\n"
	assertValue(t, input, "\"\"\"\nline1\nline2\\\n  continued\n\"\"\"", vString)
	// 内容末尾紧贴引号："""" = 内容一个 " + 闭合
	assertValue(t, "a = \"\"\"ends with \"\"\"\n", "\"\"\"ends with \"\"\"", vString)
	// 行尾反斜杠续行
	assertValue(t, "a = \"\"\"a \\\nb\"\"\"\n", "\"\"\"a \\\nb\"\"\"", vString)
}

func TestLexValueMultilineLiteral(t *testing.T) {
	assertValue(t, "a = '''\nraw\n'''\n", "'''\nraw\n'''", vString)
	assertValue(t, "a = '''C:\\now''''\n", "'''C:\\now''''", vString)
}

func TestLexValueIntegers(t *testing.T) {
	for _, c := range []struct{ raw, in string }{
		{"1", "a = 1\n"}, {"+42", "a = +42\n"}, {"-17", "a = -17\n"},
		{"0xDEAD_beef", "a = 0xDEAD_beef\n"}, {"0o755", "a = 0o755\n"},
		{"0b1101", "a = 0b1101\n"}, {"1_000_000", "a = 1_000_000\n"},
	} {
		assertValue(t, c.in, c.raw, vInteger)
	}
}

func TestLexValueFloats(t *testing.T) {
	for _, c := range []struct{ raw, in string }{
		{"3.14", "a = 3.14\n"}, {"-0.01", "a = -0.01\n"},
		{"5e+22", "a = 5e+22\n"}, {"1e6", "a = 1e6\n"},
		{"6.626e-34", "a = 6.626e-34\n"},
		{"inf", "a = inf\n"}, {"+inf", "a = +inf\n"},
		{"-inf", "a = -inf\n"}, {"nan", "a = nan\n"},
		{"1.0e1_0", "a = 1.0e1_0\n"},
	} {
		assertValue(t, c.in, c.raw, vFloat)
	}
}

func TestLexValueDatetimes(t *testing.T) {
	for _, c := range []struct{ raw, in string }{
		{"1979-05-27T07:32:00Z", "a = 1979-05-27T07:32:00Z\n"},
		{"1979-05-27T00:32:00.999999-07:00", "a = 1979-05-27T00:32:00.999999-07:00\n"},
		{"1979-05-27T07:32:00", "a = 1979-05-27T07:32:00\n"},
		{"1979-05-27 07:32:00", "a = 1979-05-27 07:32:00\n"},
		{"1979-05-27", "a = 1979-05-27\n"},
		{"07:32:00", "a = 07:32:00\n"},
		{"00:32:00.999999", "a = 00:32:00.999999\n"},
	} {
		assertValue(t, c.in, c.raw, vDatetime)
	}
}

func TestLexValueBool(t *testing.T) {
	assertValue(t, "a = true\n", "true", vBoolean)
	assertValue(t, "a = false\n", "false", vBoolean)
}

func TestLexValueInlineTable(t *testing.T) {
	assertValue(t, "a = {b = 1, c = \"x\"}\n", "{b = 1, c = \"x\"}", vInlineTable)
	// 嵌套与字符串内括号
	assertValue(t, "a = {b = {c = 1}, d = \"}\"}\n", "{b = {c = 1}, d = \"}\"}", vInlineTable)
	// 未闭合
	_, perr := lex([]byte("a = {b = 1\n"))
	if perr == nil {
		t.Fatal("未闭合内联表必须报错")
	}
}

func TestLexValueArray(t *testing.T) {
	assertValue(t, "a = [1, 2, 3]\n", "[1, 2, 3]", vArray)
	// 跨行含注释与尾逗号
	input := "a = [\n  1, # one\n  2,\n]\n"
	assertValue(t, input, "[\n  1, # one\n  2,\n]", vArray)
	// 嵌套 + 字符串内括号
	assertValue(t, "a = [[1, 2], {b = \"]\"}]\n", "[[1, 2], {b = \"]\"}]", vArray)
	// 未闭合
	_, perr := lex([]byte("a = [1, 2\n"))
	if perr == nil {
		t.Fatal("未闭合数组必须报错")
	}
	if perr.Line != 1 {
		t.Fatalf("错误行号：%d", perr.Line)
	}
	_, perr = lex([]byte("a = [1,\n2\n"))
	if perr == nil {
		t.Fatal("跨行未闭合数组必须报错")
	}
}

func TestLexValueArrayNestedStringMultiline(t *testing.T) {
	// 数组内的多行字符串
	input := "a = [\"\"\"\nx\n\"\"\", 2]\n"
	assertValue(t, input, "[\"\"\"\nx\n\"\"\", 2]", vArray)
}
