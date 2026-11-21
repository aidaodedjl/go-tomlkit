package tomlkit

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestSetReplaceInPlace(t *testing.T) {
	src := "  a = 1  # note\nb = 2\n"
	doc := mustParseOk(t, src)
	if err := doc.Set("a", int64(99)); err != nil {
		t.Fatal(err)
	}
	want := "  a = 99  # note\nb = 2\n"
	if got := doc.String(); got != want {
		t.Fatalf("原位替换不符：\n%q\nwant %q", got, want)
	}
	// 类型改变也原位
	if err := doc.Set("a", "str"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `  a = "str"  # note`) {
		t.Fatalf("类型替换不符：%q", doc.String())
	}
}

func TestSetAppendTail(t *testing.T) {
	doc := mustParseOk(t, "a = 1\nb = 2\n")
	if err := doc.Set("c", "x"); err != nil {
		t.Fatal(err)
	}
	want := "a = 1\nb = 2\nc = \"x\"\n"
	if got := doc.String(); got != want {
		t.Fatalf("表尾追加不符：\n%q\nwant %q", got, want)
	}
	// 表内追加
	doc2 := mustParseOk(t, "[t]\na = 1\n[u]\nb = 2\n")
	tb, _ := doc2.Get("t").Table()
	if err := tb.Set("new", int64(3)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc2.String(), "[t]\na = 1\nnew = 3\n") {
		t.Fatalf("表内追加位置不符：%q", doc2.String())
	}
}

func TestSetCRLFAppend(t *testing.T) {
	doc := mustParseOk(t, "a = 1\r\n")
	if err := doc.Set("b", int64(2)); err != nil {
		t.Fatal(err)
	}
	want := "a = 1\r\nb = 2\r\n"
	if got := doc.String(); got != want {
		t.Fatalf("CRLF 追加不符：%q want %q", got, want)
	}
}

func TestSetValueTypes(t *testing.T) {
	doc := mustParseOk(t, "")
	for k, v := range map[string]any{
		"i": 42, "i64": int64(-7), "f": 2.5, "s": "x", "b": true,
		"arr": []any{int64(1), "two"}, "m": map[string]any{"k": int64(1)},
	} {
		if err := doc.Set(k, v); err != nil {
			t.Fatalf("Set(%s, %T): %v", k, v, err)
		}
	}
	out := doc.String()
	for _, want := range []string{`i = 42`, `f = 2.5`, `s = "x"`, `b = true`, `arr = [1, "two"]`, `m = {k = 1}`} {
		if !strings.Contains(out, want) {
			t.Errorf("渲染缺 %q：\n%s", want, out)
		}
	}
	if err := doc.Set("bad", nil); err == nil {
		t.Error("nil 应报错")
	}
	if err := doc.Set("bad", make(chan int)); err == nil {
		t.Error("chan 应报错")
	}
	if err := doc.Set("bad", struct{}{}); err == nil {
		t.Error("struct 应报错")
	}
}

func TestSetDatetime(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	lt, _ := time.Parse("2006-01-02", "2026-11-21")
	dt := Datetime{Kind: KindLocalDate, Time: lt}
	if err := doc.Set("d", dt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `d = 2026-11-21`) {
		t.Fatalf("datetime 渲染不符：%q", doc.String())
	}
	// offset 形态
	ot, _ := time.Parse(time.RFC3339, "1979-05-27T07:32:00Z")
	odt := Datetime{Kind: KindOffsetDatetime, Time: ot}
	if err := doc.Set("o", odt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `o = 1979-05-27T07:32:00Z`) {
		t.Fatalf("offset datetime 渲染不符：%q", doc.String())
	}
}

func TestSetFloatRendering(t *testing.T) {
	doc := mustParseOk(t, "x = 1\n")
	if err := doc.Set("f", 1.0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), "f = 1.0\n") {
		t.Fatalf("1.0 渲染不符：%q", doc.String())
	}
	// parse(render(x)) == x
	rnd := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		f := rnd.Float64() * math.Pow(10, float64(rnd.Intn(30)-15))
		d2 := mustParseOk(t, "x = 1\n")
		if err := d2.Set("v", f); err != nil {
			t.Fatal(err)
		}
		re, perr := parse([]byte(d2.String()))
		if perr != nil {
			t.Fatalf("重解析失败 %v: %v", f, perr)
		}
		fv, ok := re.root.items["v"].(*Float)
		if !ok || fv.v != f {
			t.Fatalf("float 往返不等：in %v out %v", f, fv.v)
		}
	}
	// 特殊值
	d3 := mustParseOk(t, "")
	_ = d3.Set("i", math.Inf(1))
	_ = d3.Set("n", math.Inf(-1))
	_ = d3.Set("m", math.NaN())
	s := d3.String()
	for _, want := range []string{"i = inf", "n = -inf", "m = nan"} {
		if !strings.Contains(s, want) {
			t.Errorf("特殊浮点不符：%q in %q", want, s)
		}
	}
}

func TestSetStringEscapes(t *testing.T) {
	doc := mustParseOk(t, "")
	if err := doc.Set("s", "a\"b\\c\nd\te"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `"a\"b\\c\nd\te"`) {
		t.Fatalf("转义渲染不符：%q", doc.String())
	}
}

func TestSetMultilineArrayElement(t *testing.T) {
	src := "a = [\n  1, # one\n  2,\n  3,\n]\n"
	doc := mustParseOk(t, src)
	arr, ok := doc.Get("a").Array()
	if !ok {
		t.Fatal("a 不是数组")
	}
	if err := arr.Set(1, int64(99)); err != nil {
		t.Fatal(err)
	}
	want := "a = [\n  1, # one\n  99,\n  3,\n]\n"
	if got := doc.String(); got != want {
		t.Fatalf("多行数组元素替换不符：\n%q\nwant %q", got, want)
	}
}

func TestSetSingleLineArrayElement(t *testing.T) {
	doc := mustParseOk(t, "a = [1, 2, 3]\nb = 9\n")
	arr, ok := doc.Get("a").Array()
	if !ok {
		t.Fatal("a 不是数组")
	}
	if err := arr.Set(2, "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `a = [1, 2, "x"]`) {
		t.Fatalf("单行数组元素替换不符：%q", doc.String())
	}
}

func TestSetArrayWhole(t *testing.T) {
	doc := mustParseOk(t, "a = [1, 2]\n")
	if err := doc.Set("a", []any{int64(3), true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), "a = [3, true]") {
		t.Fatalf("整体数组替换不符：%q", doc.String())
	}
}

func TestSetItemCopy(t *testing.T) {
	doc := mustParseOk(t, "src = \"hello\"\ndst = \"old\"\n")
	orig := doc.Get("src")
	if err := doc.Set("dst", orig); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), `dst = "hello"`) {
		t.Fatalf("Item 拷贝不符：%q", doc.String())
	}
}
