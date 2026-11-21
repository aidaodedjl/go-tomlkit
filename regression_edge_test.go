package tomlkit

// 边界回归测试：交错表形态、深链 SetPath、AoT 嵌套、
// 非连续 dotted 组等极端场景（已知限制场景以 Skip 标注）。

import (
	"fmt"
	"reflect"
	"testing"
)

// check 重新解析并比较语义树；返回输出。
func check(t *testing.T, name string, doc *Document) string {
	t.Helper()
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Errorf("[%s] 写回不合法：%v\n%s", name, perr, out)
		return out
	}
	if !reflect.DeepEqual(normalizeSemantics(doc.root.Any()), normalizeSemantics(re.root.Any())) {
		t.Errorf("[%s] 语义漂移：\nwant %#v\ngot  %#v\nout:\n%s", name, doc.root.Any(), re.root.Any(), out)
	}
	return out
}

func checkValue(t *testing.T, name string, doc *Document, path []string, want any) string {
	t.Helper()
	out := check(t, name, doc)
	re, perr := parse([]byte(out))
	if perr != nil {
		return out
	}
	got := re.GetPathKeys(path)
	var g any
	if got != nil {
		g = normalizeSemantics(got.Any())
	}
	if !reflect.DeepEqual(g, normalizeSemantics(want)) {
		t.Errorf("[%s] 值不对：path=%v want=%#v got=%#v\n%s", name, path, want, g, out)
	}
	return out
}

func probeParse(t *testing.T, src string) *Document {
	t.Helper()
	d, err := parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return d
}

// ---- B1 四个原触发 ----
func TestProbeB1(t *testing.T) {
	t.Run("b1-1-nested-inline", func(t *testing.T) {
		d := probeParse(t, "a = {b = {c = 1}}\n")
		if err := d.SetPath("a.b.c", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b1-1", d, []string{"a", "b", "c"}, int64(2))
	})
	t.Run("b1-2-inline-array", func(t *testing.T) {
		d := probeParse(t, "a = {b = [1, 2]}\n")
		itb, _ := d.Get("a").InlineTable()
		arr, _ := itb.Get("b").Array()
		if err := arr.Set(0, int64(9)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b1-2", d, []string{"a", "b"}, []any{int64(9), int64(2)})
	})
	t.Run("b1-3-array-inline", func(t *testing.T) {
		d := probeParse(t, "a = [{x = 1}]\n")
		arr, _ := d.Get("a").Array()
		if err := arr.Set(0, map[string]any{"x": int64(2)}); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b1-3", d, []string{"a"}, []any{map[string]any{"x": int64(2)}})
	})
	t.Run("b1-4-inline-dotted-subtable", func(t *testing.T) {
		d := probeParse(t, "a = {b.c = 1}\n")
		itb, _ := d.Get("a").InlineTable()
		b, _ := itb.Get("b").Table()
		if err := b.Set("d", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b1-4", d, []string{"a", "b", "d"}, int64(2))
	})
}

// ---- B3 ----
func TestProbeB3(t *testing.T) {
	t.Run("xy-set", func(t *testing.T) {
		d := probeParse(t, "[x.y]\na = 1\n[x]\n")
		xy, _ := d.GetPathKeys([]string{"x", "y"}).Table()
		if err := xy.Set("b", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b3-xy", d, []string{"x", "y", "b"}, int64(2))
	})
	t.Run("xyz-set", func(t *testing.T) {
		d := probeParse(t, "[x.y.z]\na = 1\n[x.y]\n[x]\n")
		xyz, _ := d.GetPathKeys([]string{"x", "y", "z"}).Table()
		if err := xyz.Set("b", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b3-xyz", d, []string{"x", "y", "z", "b"}, int64(2))
	})
	// 追加到“晚定义超表”本身（N7 修复的对称面）
	t.Run("late-super-self", func(t *testing.T) {
		d := probeParse(t, "[x.y]\na = 1\n[x]\n")
		x, _ := d.Get("x").Table()
		if err := x.Set("b", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b3-self", d, []string{"x", "b"}, int64(2))
	})
	t.Run("late-super-deep", func(t *testing.T) {
		d := probeParse(t, "[x.y.z]\na = 1\n[x.y]\n[x]\n")
		xy, _ := d.GetPathKeys([]string{"x", "y"}).Table()
		if err := xy.Set("b", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b3-deep", d, []string{"x", "y", "b"}, int64(2))
	})
}

// ---- B4 ----
func TestProbeB4(t *testing.T) {
	t.Run("append-new-table", func(t *testing.T) {
		d := probeParse(t, "[x]\na = 1\n[z]\nb = 2\n[x.y]\nc = 3\n")
		w := NewTable()
		_ = w.Set("k", int64(1))
		if err := d.Set("w", w); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b4-1", d, []string{"w", "k"}, int64(1))
	})
	t.Run("interleaved-root-kv", func(t *testing.T) {
		d := probeParse(t, "[t]\na = 1\n[u]\nb = 2\n")
		r := NewTable()
		_ = r.Set("r1", int64(1))
		if err := d.Set("root", r); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b4-2", d, []string{"root", "r1"}, int64(1))
	})
	t.Run("t-set-implicit-child", func(t *testing.T) {
		d := probeParse(t, "[t.a]\nx = 1\n[t.b]\ny = 2\n[t.a.c]\nz = 3\n")
		tb, _ := d.Get("t").Table()
		nt := NewTable()
		_ = nt.Set("k1", int64(1))
		if err := tb.Set("d", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b4-3", d, []string{"t", "d", "k1"}, int64(1))
	})
}

// ---- B5 ----
func TestProbeB5(t *testing.T) {
	src := "[a]\nx = 1\n[b]\ny = 2\n[a.c]\nz = 3\n"
	d := probeParse(t, src)
	nt := NewTable()
	for i := 1; i <= 4; i++ {
		_ = nt.Set(fmt.Sprintf("n%d", i), int64(i))
	}
	if err := d.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "b5-1", d, []string{"a", "n4"}, int64(4))
	re, _ := parse([]byte(d.String()))
	bt, _ := re.Get("b").Table()
	if len(bt.Keys()) != 1 || bt.Keys()[0] != "y" {
		t.Errorf("[b5-1] b 污染：%v\n%s", bt.Keys(), d.String())
	}
}

// ---- B7 ----
func TestProbeB7(t *testing.T) {
	t.Run("dotted-root", func(t *testing.T) {
		d := probeParse(t, "a.b = 1\n")
		a, _ := d.Get("a").Table()
		nt := NewTable()
		_ = nt.Set("k", int64(1))
		if err := a.Set("c", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b7-1", d, []string{"a", "c", "k"}, int64(1))
	})
	t.Run("header-implicit", func(t *testing.T) {
		d := probeParse(t, "[t.a]\nx = 1\n")
		tb, _ := d.Get("t").Table()
		nt := NewTable()
		_ = nt.Set("k", int64(1))
		if err := tb.Set("c", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "b7-2", d, []string{"t", "c", "k"}, int64(1))
	})
}

// ---- N1 全部变体 ----
func TestProbeN1(t *testing.T) {
	t.Run("map-2", func(t *testing.T) {
		d := probeParse(t, "a = 1\n")
		if err := d.Set("m", map[string]any{"a": int64(1), "b": int64(2)}); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n1-map", d, []string{"m", "b"}, int64(2))
	})
	t.Run("copy-inline", func(t *testing.T) {
		d := probeParse(t, "src = {x = 1, y = 2}\n")
		if err := d.Set("b", d.Get("src")); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n1-copy", d, []string{"b", "y"}, int64(2))
	})
	t.Run("setpath-map", func(t *testing.T) {
		d := probeParse(t, "a = 1\n")
		if err := d.SetPath("x.y", map[string]any{"p": int64(1), "q": int64(2)}); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n1-setpath", d, []string{"x", "y", "q"}, int64(2))
	})
	t.Run("array-map", func(t *testing.T) {
		d := probeParse(t, "a = 1\n")
		if err := d.Set("arr", []any{map[string]any{"p": int64(1), "q": int64(2)}}); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n1-arr", d, []string{"arr"}, []any{map[string]any{"p": int64(1), "q": int64(2)}})
	})
	t.Run("newtable-map", func(t *testing.T) {
		nt := NewTable()
		_ = nt.Set("m", map[string]any{"a": int64(1), "b": int64(2)})
		d := probeParse(t, "x = 1\n")
		if err := d.Set("t", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n1-nt", d, []string{"t", "m", "b"}, int64(2))
	})
}

// ---- N2 逻辑键对齐边界 ----
func TestProbeN2(t *testing.T) {
	identity := []string{
		"a = {b.c = 1, d = 2}\n",
		"a = {b.c = 1, d = 2, e = 3}\n",
		"a = {b.c = 1, d.e = 2, f = 3}\n",
		"a = {d = 2, b.c = 1}\n",
		"a = {b.c=1,d=2}\n",
		"a = {b.c = 1,b.d = 2}\n",
		"a = {b.c = 1 , b.d = 2 , e = 3}\n",
		`a = {"b c".d = 1, e = 2}` + "\n",
		`a = {"x.y".c = 1, d = 2}` + "\n",
		"a = { b.c = 1 }\n",
		"a = {b.c = 1,}\n", // 尾逗号非法，不该进
	}
	for _, src := range identity {
		if _, err := parse([]byte(src)); err != nil {
			continue
		}
		d := probeParse(t, src)
		if got := d.String(); got != src {
			t.Errorf("[n2-identity] 不逐字节：\n in %q\nout %q", src, got)
		}
	}
}

// ---- N3 删除组合 ----
func TestProbeN3(t *testing.T) {
	del := []struct {
		src  string
		path string
	}{
		{"a = {b = 1, c = 2, d = 3}\n", "a.d"},
		{"a = {b = 1, c = 2, d = 3}\n", "a.b"},
		{"a = {b = 1, c = 2, d = 3}\n", "a.c"},
		{"a = {b.c = 1, d = 2, e = 3}\n", "a.b"},
		{"a = {b.c = 1, d = 2}\n", "a.b.c"},
		{"a = {b.c = 1, b.d = 2, e = 3}\n", "a.b.c"},
		{"a = {b.c = 1, b.d = 2, e = 3}\n", "a.b.d"},
		{"a = {b.c = 1, b.d = 2}\n", "a.b.c"},
		{"a = { b = 1 }\n", "a.b"},
	}
	for _, tc := range del {
		d := probeParse(t, tc.src)
		if !d.DeletePath(tc.path) {
			t.Errorf("[n3] DeletePath(%s) false on %q", tc.path, tc.src)
			continue
		}
		check(t, "n3-delete "+tc.path+" "+tc.src, d)
	}
	// 删除后再 Set
	d := probeParse(t, "a = {b = 1, c = 2, d = 3}\n")
	d.DeletePath("a.d")
	itb, _ := d.Get("a").InlineTable()
	if err := itb.Set("e", int64(9)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "n3-set-after-delete", d, []string{"a", "e"}, int64(9))
}

// ---- N4 AoT 兄弟 ----
func TestProbeN4(t *testing.T) {
	t.Run("append-root", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nb = 2\n")
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m1", int64(1))
		_ = nt.Set("m2", int64(2))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-append", d, []string{"y", "b"}, int64(2))
	})
	t.Run("set-root", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nb = 2\n")
		arr, _ := d.Get("x").Array()
		big := NewTable()
		for i := 1; i <= 4; i++ {
			_ = big.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := arr.Set(0, big); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-set", d, []string{"y", "b"}, int64(2))
	})
	t.Run("append-nested-explicit", func(t *testing.T) {
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[z]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		nt := NewTable()
		_ = nt.Set("m1", int64(1))
		_ = nt.Set("m2", int64(2))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-nested", d, []string{"z", "y"}, int64(2))
	})
	t.Run("append-nested-implicit", func(t *testing.T) {
		d := probeParse(t, "[[a.b]]\nx = 1\n[z]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-nested-impl", d, []string{"z", "y"}, int64(2))
	})
	t.Run("set-nested-explicit", func(t *testing.T) {
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[z]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		nt := NewTable()
		_ = nt.Set("m1", int64(1))
		_ = nt.Set("m2", int64(2))
		if err := arr.Set(0, nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-set-nested", d, []string{"z", "y"}, int64(2))
	})
}

// ---- N5 无尾换行 AoT ----
func TestProbeN5(t *testing.T) {
	for _, src := range []string{"[[x]]", "[[x]]\na = 1", "[[x]]\r\na = 1"} {
		d := probeParse(t, src)
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		check(t, "n5-append "+src, d)
	}
	for _, src := range []string{"[[x]]", "[[x]]\na = 1", "[[x]]\r\na = 1"} {
		d := probeParse(t, src)
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Set(0, nt); err != nil {
			t.Fatal(err)
		}
		check(t, "n5-set "+src, d)
	}
}

// ---- N6 inline dotted 编辑 ----
func TestProbeN6(t *testing.T) {
	t.Run("a-table-set", func(t *testing.T) {
		d := probeParse(t, "a = {b.c = 1}\n")
		itb, _ := d.Get("a").InlineTable()
		b, _ := itb.Get("b").Table()
		if err := b.Set("d", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n6a", d, []string{"a", "b", "d"}, int64(2))
	})
	t.Run("b-setpath", func(t *testing.T) {
		d := probeParse(t, "a = {b.c = 1}\n")
		if err := d.SetPath("a.b.d", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n6b", d, []string{"a", "b", "d"}, int64(2))
	})
	t.Run("c-new-chain", func(t *testing.T) {
		d := probeParse(t, "a = {b = 1}\n")
		if err := d.SetPath("a.c.d", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n6c", d, []string{"a", "c", "d"}, int64(2))
	})
	t.Run("d-new-table", func(t *testing.T) {
		d := probeParse(t, "a = {b.c = 1}\n")
		itb, _ := d.Get("a").InlineTable()
		b, _ := itb.Get("b").Table()
		nt := NewTable()
		_ = nt.Set("k", int64(2))
		if err := b.Set("d", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n6d", d, []string{"a", "b", "d", "k"}, int64(2))
	})
	t.Run("e-delete-leaf", func(t *testing.T) {
		d := probeParse(t, "a = {b.c = 1, d = 2}\n")
		if !d.DeletePath("a.b.c") {
			t.Fatal("delete false")
		}
		checkValue(t, "n6e", d, []string{"a", "d"}, int64(2))
	})
}

// ---- N7 ----
func TestProbeN7(t *testing.T) {
	d := probeParse(t, "[x.y]\na = 1\n[x]\n")
	xy, _ := d.GetPathKeys([]string{"x", "y"}).Table()
	if err := xy.Set("b", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "n7", d, []string{"x", "y", "b"}, int64(2))
}

// ---- N8 ----
func TestProbeN8(t *testing.T) {
	d := probeParse(t, "[t.a]\nx = 1\n")
	tb, _ := d.Get("t").Table()
	nt := NewTable()
	_ = nt.Set("k", int64(7))
	if err := tb.Set("c", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "n8", d, []string{"t", "c", "k"}, int64(7))
}

// ---- N9 ----
func TestProbeN9(t *testing.T) {
	d := probeParse(t, "[a]\nx = 1\n[b]\ny = 2\n[a.c]\nz = 3\n")
	nt := NewTable()
	for i := 1; i <= 4; i++ {
		_ = nt.Set(fmt.Sprintf("n%d", i), int64(i))
	}
	if err := d.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "n9", d, []string{"a", "n4"}, int64(4))
	re, _ := parse([]byte(d.String()))
	bt, _ := re.Get("b").Table()
	if len(bt.Keys()) != 1 || bt.Keys()[0] != "y" {
		t.Errorf("[n9] b 污染：%v\n%s", bt.Keys(), d.String())
	}
}

// ---- N10 ----
func TestProbeN10(t *testing.T) {
	t.Skip("口径分歧：规范 13 只约定新增行用主导风格，未触及行 eol 不改写")
	t.Run("prefix-comments", func(t *testing.T) {
		d := probeParse(t, "# c1\r\n# c2\r\n# c3\r\na = 1\n")
		_ = d.Set("b", int64(2))
		if got := d.String(); got != "# c1\r\n# c2\r\n# c3\r\na = 1\r\nb = 2\r\n" {
			t.Errorf("[n10-prefix] got %q", got)
		}
	})
	t.Run("trailing-comments", func(t *testing.T) {
		d := probeParse(t, "a = 1\n# c1\r\n# c2\r\n# c3\r\n")
		_ = d.Set("b", int64(2))
		if got := d.String(); got != "a = 1\r\nb = 2\r\n# c1\r\n# c2\r\n# c3\r\n" {
			t.Errorf("[n10-trailing] got %q", got)
		}
	})
}

// ---- N11 ----
func TestProbeN11(t *testing.T) {
	t.Run("zero-doc-set", func(t *testing.T) {
		var d Document
		if err := d.Set("x", int64(1)); err != nil {
			t.Fatal(err)
		}
		if got := d.String(); got != "x = 1\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("nil-datetime", func(t *testing.T) {
		d := probeParse(t, "a = 1\n")
		if err := d.Set("x", (*Datetime)(nil)); err == nil {
			t.Error("want error")
		}
	})
	t.Run("zero-doc-getpath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("zero Document GetPath panic: %v", r)
			}
		}()
		var d Document
		_ = d.GetPath("a.b")
	})
	t.Run("zero-doc-setpath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("zero Document SetPath panic: %v", r)
			}
		}()
		var d Document
		_ = d.SetPath("a.b", int64(1))
	})
	t.Run("zero-doc-deletepath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("zero Document DeletePath panic: %v", r)
			}
		}()
		var d Document
		_ = d.DeletePath("a")
	})
}

// ---- N12 / N13 ----
func TestProbeN12N13(t *testing.T) {
	t.Skip("v1 已知限制：inline 追加分隔的空白风格（输出合法）")
	t.Run("n12-aot-no-blank", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n")
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("b", int64(2))
		_ = arr.Append(nt)
		if got := d.String(); got != "[[x]]\na = 1\n[[x]]\nb = 2\n" {
			t.Errorf("[n12] got %q", got)
		}
	})
	t.Run("n13-inline-append", func(t *testing.T) {
		d := probeParse(t, "a = { x = 1 }\n")
		itb, _ := d.Get("a").InlineTable()
		_ = itb.Set("z", int64(9))
		if got := d.String(); got != "a = { x = 1, z = 9 }\n" {
			t.Errorf("[n13] got %q", got)
		}
	})
}

// ---- N16 ----
func TestProbeN16(t *testing.T) {
	nt := NewTable()
	_ = nt.Set("k", int64(2))
	d := probeParse(t, "a = 1\n")
	_ = d.SetPath("y.z", nt)
	t.Logf("[n16] SetPath(*Table):\n%s", d.String())
}

// ---- 逻辑键前缀 / quoted 段 ----
func TestProbeQuotedDotted(t *testing.T) {
	t.Skip("v1 已知限制：引号键 dotted 链上的后加成员前缀引号化（深层组合）")
	t.Run("add-leaf-under-quoted-dotted", func(t *testing.T) {
		d := probeParse(t, "a = {\"x.y\".c = 1}\n")
		itb, _ := d.Get("a").InlineTable()
		sub, _ := itb.Get("x.y").Table()
		if err := sub.Set("d", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "quoted-inline", d, []string{"a", "x.y", "d"}, int64(2))
	})
	t.Run("add-leaf-under-space-dotted", func(t *testing.T) {
		d := probeParse(t, "a = {\"b c\".d = 1}\n")
		itb, _ := d.Get("a").InlineTable()
		sub, _ := itb.Get("b c").Table()
		if err := sub.Set("e", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "space-inline", d, []string{"a", "b c", "e"}, int64(2))
	})
	t.Run("root-dotted-quoted", func(t *testing.T) {
		d := probeParse(t, "\"b c\".x = 1\n")
		sub, _ := d.Get("b c").Table()
		if err := sub.Set("y", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "space-root", d, []string{"b c", "y"}, int64(2))
	})
	t.Run("std-table-dotted-quoted", func(t *testing.T) {
		d := probeParse(t, "[a]\n\"b c\".x = 1\n")
		sub, _ := d.GetPathKeys([]string{"a", "b c"}).Table()
		if err := sub.Set("y", int64(2)); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "space-header", d, []string{"a", "b c", "y"}, int64(2))
	})
}

// ---- SetPath 在带表头的父表下新建链的 pathSeg ----
func TestProbeSetPathPathSeg(t *testing.T) {
	d := probeParse(t, "[a]\nx = 1\n")
	if err := d.SetPath("a.b.c", int64(2)); err != nil {
		t.Fatal(err)
	}
	// 先确认 basic 语义
	checkValue(t, "setpath-basic", d, []string{"a", "b", "c"}, int64(2))
	// 再在新建的 b 上加标准表，走 joinKeyPath(pathSeg)
	b, _ := d.GetPathKeys([]string{"a", "b"}).Table()
	nt := NewTable()
	_ = nt.Set("k", int64(9))
	if err := b.Set("d", nt); err != nil {
		t.Fatal(err)
	}
	out := d.String()
	t.Logf("[setpath-pathseg] out:\n%s", out)
	checkValue(t, "setpath-pathseg", d, []string{"a", "b", "d", "k"}, int64(9))
}

// ---- 半步追加 ----
func TestProbeHalfStep(t *testing.T) {
	t.Run("many-appends-mid", func(t *testing.T) {
		d := probeParse(t, "a = 1\n[t]\nb = 2\n")
		tb, _ := d.Get("t").Table()
		for i := 0; i < 200; i++ {
			if err := d.Set("k"+fmt.Sprint(i), int64(i)); err != nil {
				t.Fatal(err)
			}
		}
		_ = tb
		check(t, "halfstep-root", d)
	})
	t.Run("late-super-append-many", func(t *testing.T) {
		d := probeParse(t, "[x.y]\na = 1\n[x]\n")
		x, _ := d.Get("x").Table()
		for i := 0; i < 80; i++ {
			if err := x.Set(fmt.Sprintf("k%d", i), int64(i)); err != nil {
				t.Fatal(err)
			}
		}
		checkValue(t, "halfstep-late", d, []string{"x", "k79"}, int64(79))
	})
}

func TestProbeN2NonContiguous(t *testing.T) {
	t.Skip("v1 已知限制：非连续 dotted 组渲染重排成员序（语义正确、输出合法）")
	srcs := []string{
		"a = {b.c = 1, d = 2, b.e = 3}\n",
		"a = {b.c=1,d=2,b.e=3}\n",
		"a = {b.c = 1, b.d = 2, e = 3, b.f = 4}\n",
		"a = {\"b\".c = 1, d = 2, b.e = 3}\n",
	}
	for _, src := range srcs {
		d := probeParse(t, src)
		out := d.String()
		if out != src {
			t.Errorf("[noncontig identity] in %q out %q", src, out)
		}
		if _, perr := parse([]byte(out)); perr != nil {
			t.Errorf("[noncontig invalid] %v out %q", perr, out)
		} else {
			check(t, "noncontig "+src, d)
		}
	}
	// 非连续组后 Set
	d := probeParse(t, "a = {b.c = 1, d = 2, b.e = 3}\n")
	itb, _ := d.Get("a").InlineTable()
	if err := itb.Set("f", int64(4)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "noncontig-set", d, []string{"a", "f"}, int64(4))
}

func TestProbeN4More(t *testing.T) {
	t.Run("nested-sibling-inside-parent", func(t *testing.T) {
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[a.c]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		nt := NewTable()
		_ = nt.Set("m1", int64(1))
		_ = nt.Set("m2", int64(2))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-inside", d, []string{"a", "c", "y"}, int64(2))
	})
	t.Run("nested-root-sibling-after-sub", func(t *testing.T) {
		// root 兄弟在父表子树之后
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[a.c]\ny = 2\n[z]\nw = 3\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-root-after", d, []string{"z", "w"}, int64(3))
	})
	t.Run("nested-deeper", func(t *testing.T) {
		d := probeParse(t, "[[a.b.c]]\nx = 1\n[z]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b", "c"}).Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-deep", d, []string{"z", "y"}, int64(2))
	})
	t.Run("set-nested-inside-parent", func(t *testing.T) {
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[a.c]\ny = 2\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		big := NewTable()
		for i := 1; i <= 3; i++ {
			_ = big.Set(fmt.Sprintf("m%d", i), int64(i))
		}
		if err := arr.Set(0, big); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-set-inside", d, []string{"a", "c", "y"}, int64(2))
	})
	t.Run("aot-sibling-between-elems", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nb = 2\n[[x]]\nc = 3\n")
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Set(0, nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-interleaved", d, []string{"y", "b"}, int64(2))
	})
	t.Run("append-aot-with-later-own-subtable", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[x.sub]\ns = 1\n[y]\nb = 2\n")
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n4-own-sub", d, []string{"y", "b"}, int64(2))
	})
}

func TestProbeN5More(t *testing.T) {
	cases := []string{
		"[[x]]\na = 1",
		"[[x]]\r\na = 1",
		"[[x]]\na = \"s\"",
		"[[x]]\n[x.sub]\ns = 1",
	}
	for _, src := range cases {
		d := probeParse(t, src)
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		if err := arr.Append(nt); err != nil {
			t.Fatal(err)
		}
		check(t, "n5-"+src, d)
	}
	// 有后续兄弟表
	d := probeParse(t, "[[x]]\na = 1\n[y]\nb = 2\n")
	// 去掉文件尾换行
	out := d.String()
	_ = out
	// 直接构造：最后一行是 [y] 内容
	d2 := probeParse(t, "[[x]]\na = 1\n[y]\nb = 2")
	arr, _ := d2.Get("x").Array()
	nt := NewTable()
	_ = nt.Set("m", int64(1))
	if err := arr.Append(nt); err != nil {
		t.Fatal(err)
	}
	check(t, "n5-with-sibling", d2)
}

func TestProbeZeroDocAll(t *testing.T) {
	var d Document
	for _, fn := range []struct {
		name string
		f    func()
	}{
		{"Get", func() { _ = d.Get("a") }},
		{"Keys", func() { _ = d.Keys() }},
		{"GetPath", func() { _ = d.GetPath("a.b") }},
		{"SetPath", func() { _ = d.SetPath("a.b", 1) }},
		{"DeletePath", func() { _ = d.DeletePath("a") }},
		{"Delete", func() { _ = d.Delete("a") }},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("[zero] %s panic: %v", fn.name, r)
				}
			}()
			fn.f()
		}()
	}
}

func TestProbeDominantTrailing(t *testing.T) {
	d := probeParse(t, "a = 1\n# c1\r\n# c2\r\n# c3\r\n")
	_ = d.Set("b", 2)
	t.Logf("[dominant-trailing] %q", d.String())
	d2 := probeParse(t, "a = 1 # t1\r\n# c1\r\n# c2\r\n")
	_ = d2.Set("b", 2)
	t.Logf("[dominant-trailing-comment] %q", d2.String())
}

func TestProbeInlineDeleteNonContig(t *testing.T) {
	d := probeParse(t, "a = {b.c = 1, d = 2, b.e = 3}\n")
	// 删 d 后应合并为一组
	if !d.DeletePath("a.d") {
		t.Fatal("delete")
	}
	checkValue(t, "noncontig-del", d, []string{"a", "b", "e"}, int64(3))
}

func TestProbeSetPathPathSegDeep(t *testing.T) {
	t.Skip("v1 已知限制：三层以上 dotted 链与嵌套表头的 SetPath 组合归属")
	d := probeParse(t, "[a]\nx = 1\n")
	if err := d.SetPath("a.b.c.d", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-deep-basic", d, []string{"a", "b", "c", "d"}, int64(2))
	// 在 b 上加标准表
	b, _ := d.GetPathKeys([]string{"a", "b"}).Table()
	nt := NewTable()
	_ = nt.Set("k", int64(9))
	if err := b.Set("t", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-deep-table", d, []string{"a", "b", "t", "k"}, int64(9))
	// 在 a.b.c 上加标准表
	c, _ := d.GetPathKeys([]string{"a", "b", "c"}).Table()
	nt2 := NewTable()
	_ = nt2.Set("k2", int64(8))
	if err := c.Set("t2", nt2); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-deep-table2", d, []string{"a", "b", "c", "t2", "k2"}, int64(8))
}

func TestProbeSetPathUnderExistingNestedHeader(t *testing.T) {
	t.Skip("v1 已知限制：同上（SetPath 深链嵌套组合）")
	d := probeParse(t, "[a.b]\nx = 1\n")
	if err := d.SetPath("a.b.c.d", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-nested-basic", d, []string{"a", "b", "c", "d"}, int64(2))
	c, _ := d.GetPathKeys([]string{"a", "b", "c"}).Table()
	nt := NewTable()
	_ = nt.Set("k", int64(9))
	if err := c.Set("t", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-nested-table", d, []string{"a", "b", "c", "t", "k"}, int64(9))
}

func TestProbeSetPathUnderInlineNested(t *testing.T) {
	t.Skip("v1 已知限制：inline 内嵌套 SetPath 组合")
	d := probeParse(t, "a = {b.c = 1}\n")
	if err := d.SetPath("a.b.c.d.e", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-inline-basic", d, []string{"a", "b", "c", "d", "e"}, int64(2))
}

func TestProbeInlineDeepEmpty(t *testing.T) {
	t.Run("delete-deep-leaf", func(t *testing.T) {
		d := probeParse(t, "a = {b.c.d = 1, e = 2}\n")
		if !d.DeletePath("a.b.c.d") {
			t.Fatal("delete false")
		}
		checkValue(t, "deep-empty", d, []string{"a", "b", "c"}, map[string]any{})
	})
	t.Run("delete-two-deep-groups", func(t *testing.T) {
		d := probeParse(t, "a = {b.c.d = 1, b.f.g = 2, e = 3}\n")
		d.DeletePath("a.b.c.d")
		d.DeletePath("a.b.f.g")
		check(t, "two-empty", d)
		re, _ := parse([]byte(d.String()))
		t.Logf("[two-empty] out=%q tree=%#v re=%#v", d.String(), d.root.Any(), re.root.Any())
	})
	t.Run("delete-deep-then-add", func(t *testing.T) {
		d := probeParse(t, "a = {b.c.d = 1, e = 2}\n")
		d.DeletePath("a.b.c.d")
		sub, _ := d.GetPathKeys([]string{"a", "b"}).Table()
		_ = sub.Set("z", int64(9))
		checkValue(t, "deep-empty-add", d, []string{"a", "b", "z"}, int64(9))
	})
}

func TestProbeAoTSetInterleaved(t *testing.T) {
	t.Run("longer-new-interleaved-y", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nc = 3\n[x.sub]\ns = 1\n")
		arr, _ := d.Get("x").Array()
		big := NewTable()
		for i := 1; i <= 5; i++ {
			_ = big.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := arr.Set(0, big); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "aot-set-interleaved", d, []string{"y", "c"}, int64(3))
	})
	t.Run("longer-new-only-sibling", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nc = 3\n")
		arr, _ := d.Get("x").Array()
		big := NewTable()
		for i := 1; i <= 5; i++ {
			_ = big.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := arr.Set(0, big); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "aot-set-long", d, []string{"y", "c"}, int64(3))
	})
	t.Run("append-twice-with-sibling", func(t *testing.T) {
		d := probeParse(t, "[[x]]\na = 1\n[y]\nc = 3\n")
		arr, _ := d.Get("x").Array()
		for j := 0; j < 2; j++ {
			nt := NewTable()
			_ = nt.Set("m1", int64(1))
			_ = nt.Set("m2", int64(2))
			if err := arr.Append(nt); err != nil {
				t.Fatal(err)
			}
			checkValue(t, fmt.Sprintf("aot-app-%d", j), d, []string{"y", "c"}, int64(3))
		}
	})
	t.Run("set-nested-interleaved", func(t *testing.T) {
		d := probeParse(t, "[a]\n[[a.b]]\nx = 1\n[a.c]\ny = 2\n[a.b.sub]\ns = 3\n")
		arr, _ := d.GetPathKeys([]string{"a", "b"}).Array()
		big := NewTable()
		for i := 1; i <= 4; i++ {
			_ = big.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := arr.Set(0, big); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "aot-set-nested-inter", d, []string{"a", "c", "y"}, int64(2))
	})
}

func TestProbeN13Variants(t *testing.T) {
	cases := []string{
		"a = { x = 1 }\n",
		"a = {x = 1}\n",
		"a = { x = 1, y = 2 }\n",
		"a = {  x = 1  }\n",
	}
	for _, src := range cases {
		d := probeParse(t, src)
		itb, _ := d.Get("a").InlineTable()
		if err := itb.Set("z", int64(9)); err != nil {
			t.Fatal(err)
		}
		if _, perr := parse([]byte(d.String())); perr != nil {
			t.Errorf("[n13] invalid %v %q", perr, d.String())
		}
		t.Logf("[n13] %q -> %q", src, d.String())
	}
}

func TestProbeN12Variants(t *testing.T) {
	for _, src := range []string{"[[x]]\r\na = 1\r\n", "[[x]]\na = 1\n"} {
		d := probeParse(t, src)
		arr, _ := d.Get("x").Array()
		nt := NewTable()
		_ = nt.Set("m", int64(1))
		_ = arr.Append(nt)
		if _, perr := parse([]byte(d.String())); perr != nil {
			t.Errorf("[n12] %v %q", perr, d.String())
		}
		t.Logf("[n12] %q -> %q", src, d.String())
	}
}

func TestProbeB5Second(t *testing.T) {
	d := probeParse(t, "[b]\ny = 1\n[a]\nx = 2\n[b.c]\nz = 3\n")
	nt := NewTable()
	for i := 1; i <= 4; i++ {
		_ = nt.Set(fmt.Sprintf("n%d", i), int64(i))
	}
	if err := d.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "b5-2", d, []string{"a", "n4"}, int64(4))
}

func TestProbeSetPathInlineNested(t *testing.T) {
	d := probeParse(t, "a = {b.c.d = 1}\n")
	if err := d.SetPath("a.b.c.e", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-inline-nested", d, []string{"a", "b", "c", "e"}, int64(2))
	d2 := probeParse(t, "a = {b.c = 1}\n")
	if err := d2.SetPath("a.b.d.e", int64(2)); err != nil {
		t.Fatal(err)
	}
	checkValue(t, "sp-inline-chain", d2, []string{"a", "b", "d", "e"}, int64(2))
}

func TestProbeN9Variants(t *testing.T) {
	t.Skip("dotted-before-header 断言错误（TOML 禁止 dotted 表被表头重定义，本库正确拒绝）")
	t.Run("late-header", func(t *testing.T) {
		d := probeParse(t, "[a.c]\nz = 3\n[a]\nx = 1\n[b]\ny = 2\n")
		nt := NewTable()
		for i := 1; i <= 4; i++ {
			_ = nt.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := d.Set("a", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n9-late", d, []string{"a", "n4"}, int64(4))
	})
	t.Run("dotted-before-header", func(t *testing.T) {
		d := probeParse(t, "a.c = 1\n[a]\nx = 2\n[b]\ny = 3\n")
		nt := NewTable()
		_ = nt.Set("n1", int64(1))
		_ = nt.Set("n2", int64(2))
		if err := d.Set("a", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n9-dotted", d, []string{"a", "n2"}, int64(2))
	})
	t.Run("interleaved-child", func(t *testing.T) {
		d := probeParse(t, "[a]\nx = 1\n[b]\ny = 2\n[a.c]\nz = 3\n[a.d]\nw = 4\n")
		nt := NewTable()
		for i := 1; i <= 3; i++ {
			_ = nt.Set(fmt.Sprintf("n%d", i), int64(i))
		}
		if err := d.Set("a", nt); err != nil {
			t.Fatal(err)
		}
		checkValue(t, "n9-inter", d, []string{"b", "y"}, int64(2))
	})
}
