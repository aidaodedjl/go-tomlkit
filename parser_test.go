package tomlkit

import (
	"math"
	"testing"
)

func mustParseOk(t *testing.T, src string) *Document {
	t.Helper()
	doc, perr := parse([]byte(src))
	if perr != nil {
		t.Fatalf("解析失败：%v", perr)
	}
	return doc
}

func TestParseBasic(t *testing.T) {
	doc := mustParseOk(t, "a = 1\nb = \"x\"\nc = true\n")
	if got := doc.root.keys; len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("根表键序不符：%v", got)
	}
	a := doc.root.items["a"].(*Integer)
	if a.v != 1 || a.raw != "1" {
		t.Fatalf("a 不符：%+v", a)
	}
	b := doc.root.items["b"].(*String)
	if b.v != "x" {
		t.Fatalf("b 不符：%+v", b)
	}
	c := doc.root.items["c"].(*Boolean)
	if !c.v {
		t.Fatalf("c 不符：%+v", c)
	}
}

func TestParseDecor(t *testing.T) {
	doc := mustParseOk(t, "\n# c\n  a  =  1  # tail\n")
	a := doc.root.items["a"].(*Integer)
	if a.d.prefix != "\n# c\n  " {
		t.Fatalf("prefix 不符：%q", a.d.prefix)
	}
	if a.d.keyRaw != "a" || a.d.sep != "  =  " {
		t.Fatalf("keyRaw/sep 不符：%q %q", a.d.keyRaw, a.d.sep)
	}
	if a.d.tail != "  # tail" {
		t.Fatalf("tail 不符：%q", a.d.tail)
	}
	if a.d.eol != "\n" {
		t.Fatalf("eol 不符：%q", a.d.eol)
	}
}

func TestParseEOLVariants(t *testing.T) {
	doc := mustParseOk(t, "a = 1")
	if a := doc.root.items["a"].(*Integer); a.d.eol != "" {
		t.Fatalf("无尾换行 eol 应为空：%q", a.d.eol)
	}
	doc = mustParseOk(t, "a = 1\r\n")
	if a := doc.root.items["a"].(*Integer); a.d.eol != "\r\n" {
		t.Fatalf("CRLF eol 不符：%q", a.d.eol)
	}
}

func TestParseTableHeader(t *testing.T) {
	doc := mustParseOk(t, "[a]\nx = 1\n[a.b]\ny = 2\n")
	a := doc.root.items["a"].(*Table)
	if !a.hasHeader || len(a.keys) != 2 || a.keys[0] != "x" || a.keys[1] != "b" {
		t.Fatalf("a 不符：%+v", a)
	}
	ab := a.items["b"].(*Table)
	if !ab.hasHeader || ab.d.keyRaw != "a.b" {
		t.Fatalf("a.b 不符：%+v", ab)
	}
	if y := ab.items["y"].(*Integer); y.v != 2 {
		t.Fatalf("y 不符：%+v", y)
	}
	// a 表头的 decor
	if a.d.keyRaw != "a" || a.d.eol != "\n" {
		t.Fatalf("a 表头 decor 不符：%+v", a.d)
	}
}

func TestParseDuplicateKV(t *testing.T) {
	_, perr := parse([]byte("a = 1\na = 2\n"))
	if perr == nil {
		t.Fatal("重复键必须报错")
	}
	if perr.Line != 2 || perr.Snippet != "a = 2" {
		t.Fatalf("错误位置不符：%+v", perr)
	}
}

func TestParseDuplicateTable(t *testing.T) {
	_, perr := parse([]byte("[a]\nx = 1\n[a]\ny = 2\n"))
	if perr == nil {
		t.Fatal("重复表必须报错")
	}
	if perr.Line != 3 {
		t.Fatalf("错误行号：%d", perr.Line)
	}
}

func TestParseKeyConflictsWithTable(t *testing.T) {
	if _, perr := parse([]byte("a = 1\n[a]\n")); perr == nil {
		t.Fatal("标量后定义同名表必须报错")
	}
	if _, perr := parse([]byte("[a]\nx = 1\nx.y = 2\n")); perr == nil {
		t.Fatal("已有键上 dotted 定义必须报错")
	}
}

func TestParseStringDecoding(t *testing.T) {
	doc := mustParseOk(t, "a = \"b\\tc\\n\\\"\\\\\"\nb = 'lit\\eral'\n")
	if doc.root.items["a"].(*String).v != "b\tc\n\"\\" {
		t.Fatalf("基本串解码不符：%q", doc.root.items["a"].(*String).v)
	}
	if doc.root.items["b"].(*String).v != "lit\\eral" {
		t.Fatalf("字面串解码不符：%q", doc.root.items["b"].(*String).v)
	}
}

func TestParseMultilineStringDecoding(t *testing.T) {
	doc := mustParseOk(t, "a = \"\"\"\nfirst\nsecond\"\"\"\n")
	if v := doc.root.items["a"].(*String).v; v != "first\nsecond" {
		t.Fatalf("多行串首换行剥离不符：%q", v)
	}
	doc = mustParseOk(t, "a = \"\"\"one \\\n  two\"\"\"\n")
	if v := doc.root.items["a"].(*String).v; v != "one two" {
		t.Fatalf("续行折叠不符：%q", v)
	}
}

func TestParseIntegers(t *testing.T) {
	for _, c := range []struct {
		src string
		val int64
	}{
		{"a = 99\n", 99}, {"a = +42\n", 42}, {"a = -17\n", -17},
		{"a = 0xDEAD_beef\n", 0xDEADBEEF}, {"a = 0o755\n", 0755},
		{"a = 0b1101\n", 13}, {"a = 1_000_000\n", 1000000},
	} {
		doc := mustParseOk(t, c.src)
		if v := doc.root.items["a"].(*Integer).v; v != c.val {
			t.Fatalf("%q 解码不符：got %d want %d", c.src, v, c.val)
		}
	}
	if _, perr := parse([]byte("a = 9223372036854775808\n")); perr == nil {
		t.Fatal("int64 溢出必须报错")
	}
}

func TestParseFloats(t *testing.T) {
	doc := mustParseOk(t, "a = 3.14\nb = inf\nc = -inf\nd = nan\ne = 1e6\n")
	if doc.root.items["a"].(*Float).v != 3.14 {
		t.Fatal("3.14")
	}
	b := doc.root.items["b"].(*Float).v
	if !math.IsInf(b, 1) {
		t.Fatal("inf")
	}
	if !math.IsInf(doc.root.items["c"].(*Float).v, -1) {
		t.Fatal("-inf")
	}
	if !math.IsNaN(doc.root.items["d"].(*Float).v) {
		t.Fatal("nan")
	}
	if doc.root.items["e"].(*Float).v != 1e6 {
		t.Fatal("1e6")
	}
}

func TestParseErrorImplementsError(t *testing.T) {
	_, perr := parse([]byte("a = \n"))
	if perr == nil {
		t.Fatal("缺值必须报错")
	}
	var _ error = perr
}
