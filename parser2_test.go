package tomlkit

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseDottedKeys(t *testing.T) {
	doc := mustParseOk(t, "a.b.c = 1\n")
	a := doc.root.items["a"].(*Table)
	if !a.implicit || !a.dottedCreated {
		t.Fatalf("a 应为 dotted 创建的隐式表：%+v", a)
	}
	b := a.items["b"].(*Table)
	if !b.dottedCreated {
		t.Fatalf("b 应为 dotted 创建")
	}
	c := b.items["c"].(*Integer)
	if c.v != 1 || !c.d.dotted || c.d.keyRaw != "a.b.c" {
		t.Fatalf("叶子不符：%+v", c)
	}
}

func TestParseDottedHeaderConflicts(t *testing.T) {
	if _, perr := parse([]byte("x.y = 1\n[x]\nz = 2\n")); perr == nil {
		t.Fatal("dotted 创建的表被表头定义必须报错")
	}
	mustParseOk(t, "[a]\nb.c = 1\n") // 显式表内 dotted 合法
	mustParseOk(t, "[p.q]\n[p]\n")   // 隐式中间表转显式合法
	mustParseOk(t, "x.y = 1\nx.z = 2\n")
	if _, perr := parse([]byte("a.b = 1\n[a.b.c]\nx = 1\n")); perr == nil {
		t.Fatal("表头路径穿过 dotted 创建的表必须报错")
	}
}

func TestParseInlineTable(t *testing.T) {
	doc := mustParseOk(t, "a = {b = 1, c = \"x\", d.e = 2}\n")
	it := doc.root.items["a"].(*InlineTable)
	if len(it.keys) != 3 || it.keys[0] != "b" || it.keys[1] != "c" || it.keys[2] != "d" {
		t.Fatalf("内联表键序不符：%v", it.keys)
	}
	if it.items["b"].(*Integer).v != 1 || it.items["c"].(*String).v != "x" {
		t.Fatal("成员不符")
	}
	d := it.items["d"].(*Table)
	if d.items["e"].(*Integer).v != 2 {
		t.Fatal("内联表 dotted 成员不符")
	}
	if len(it.Keys()) != 3 {
		t.Fatalf("内联表成员不符：%v", it.Keys())
	}
}

func TestParseInlineTableErrors(t *testing.T) {
	if _, perr := parse([]byte("a = {b = 1, b = 2}\n")); perr == nil {
		t.Fatal("内联表重复键必须报错")
	}
	if _, perr := parse([]byte("a = {b = 1\n}\n")); perr == nil {
		t.Fatal("TOML 1.0 内联表禁止换行")
	}
}

func TestParseInlineTableNested(t *testing.T) {
	doc := mustParseOk(t, "a = {b = {c = 1}, d = [1, 2]}\n")
	it := doc.root.items["a"].(*InlineTable)
	if it.items["b"].(*InlineTable).items["c"].(*Integer).v != 1 {
		t.Fatal("嵌套内联表不符")
	}
	if arr := it.items["d"].(*Array); len(arr.items) != 2 {
		t.Fatal("内联表内数组不符")
	}
}

func TestParseArray(t *testing.T) {
	doc := mustParseOk(t, "a = [1, \"two\", true]\n")
	arr := doc.root.items["a"].(*Array)
	if len(arr.items) != 3 {
		t.Fatalf("数组长度不符：%d", len(arr.items))
	}
	if arr.items[0].(*Integer).v != 1 || arr.items[1].(*String).v != "two" || !arr.items[2].(*Boolean).v {
		t.Fatal("数组元素不符")
	}
	if len(arr.items) != 3 {
		t.Fatal("数组元素不符")
	}
}

func TestParseArrayMultiline(t *testing.T) {
	doc := mustParseOk(t, "a = [\n  1, # one\n  2,\n]\n")
	arr := doc.root.items["a"].(*Array)
	if len(arr.items) != 2 || arr.items[0].(*Integer).v != 1 || arr.items[1].(*Integer).v != 2 {
		t.Fatal("多行数组元素不符")
	}
	if len(arr.elems) != 2 {
		t.Fatalf("elems 数不符：%d", len(arr.elems))
	}
	// 元素上下文：首元素 prefix 含 [ 后换行与缩进
	if arr.elems[0].prefix != "\n  " {
		t.Fatalf("elems[0].prefix 不符：%q", arr.elems[0].prefix)
	}
	if arr.elems[1].prefix != " # one\n  " {
		t.Fatalf("elems[1].prefix 不符：%q", arr.elems[1].prefix)
	}
}

func TestParseArrayNested(t *testing.T) {
	doc := mustParseOk(t, "a = [[1, 2], {b = 3}]\n")
	arr := doc.root.items["a"].(*Array)
	inner := arr.items[0].(*Array)
	if len(inner.items) != 2 || inner.items[0].(*Integer).v != 1 {
		t.Fatal("嵌套数组不符")
	}
	if arr.items[1].(*InlineTable).items["b"].(*Integer).v != 3 {
		t.Fatal("数组内内联表不符")
	}
}

func TestParseAoT(t *testing.T) {
	doc := mustParseOk(t, "[[fruit]]\nname = \"apple\"\n[[fruit]]\nname = \"banana\"\n")
	arr := doc.root.items["fruit"].(*Array)
	if !arr.aot || len(arr.items) != 2 {
		t.Fatalf("AoT 不符：%+v", arr)
	}
	f0 := arr.items[0].(*Table)
	if f0.items["name"].(*String).v != "apple" {
		t.Fatal("AoT 元素 0 不符")
	}
	if f0.d.keyRaw != "fruit" {
		t.Fatalf("AoT 元素表头 keyRaw 不符：%q", f0.d.keyRaw)
	}
}

func TestParseAoTDeepPath(t *testing.T) {
	doc := mustParseOk(t, "[[a.b]]\nx = 1\n")
	a := doc.root.items["a"].(*Table)
	if !a.implicit {
		t.Fatal("深路径中间段应为隐式表")
	}
	arr := a.items["b"].(*Array)
	if !arr.aot || len(arr.items) != 1 {
		t.Fatal("深路径 AoT 不符")
	}
}

func TestParseDatetimeKinds(t *testing.T) {
	cases := []struct {
		src  string
		kind DatetimeKind
		want time.Time
	}{
		{"a = 1979-05-27T07:32:00Z\n", KindOffsetDatetime, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)},
		{"a = 1979-05-27T00:32:00.999999-07:00\n", KindOffsetDatetime,
			time.Date(1979, 5, 27, 0, 32, 0, 999999000, time.FixedZone("", -7*3600))},
		{"a = 1979-05-27T07:32:00\n", KindLocalDatetime, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)},
		{"a = 1979-05-27 07:32:00\n", KindLocalDatetime, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)},
		{"a = 1979-05-27\n", KindLocalDate, time.Date(1979, 5, 27, 0, 0, 0, 0, time.UTC)},
		{"a = 07:32:00\n", KindLocalTime, time.Date(0, 1, 1, 7, 32, 0, 0, time.UTC)},
		{"a = 00:32:00.999999\n", KindLocalTime, time.Date(0, 1, 1, 0, 32, 0, 999999000, time.UTC)},
	}
	for _, c := range cases {
		doc, perr := parse([]byte(c.src))
		if perr != nil {
			t.Fatalf("%q 解析失败：%v", c.src, perr)
		}
		dt := doc.root.items["a"].(*Datetime)
		if dt.Kind != c.kind {
			t.Fatalf("%q kind 不符：got %v want %v", c.src, dt.Kind, c.kind)
		}
		if !dt.Time.Equal(c.want) {
			t.Fatalf("%q 时间不符：got %v want %v", c.src, dt.Time, c.want)
		}
	}
}

func TestParseDeepDotted(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 999; i++ {
		sb.WriteString("k")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(".")
	}
	sb.WriteString("leaf = 1\n")
	doc, perr := parse([]byte(sb.String()))
	if perr != nil {
		t.Fatalf("深嵌套解析失败：%v", perr)
	}
	cur := doc.root
	for i := 0; i < 999; i++ {
		name := "k" + strconv.Itoa(i)
		next, ok := cur.items[name].(*Table)
		if !ok {
			t.Fatalf("第 %d 层 %s 不是表", i, name)
		}
		cur = next
	}
	if cur.items["leaf"].(*Integer).v != 1 {
		t.Fatal("深嵌套叶子不符")
	}
}
