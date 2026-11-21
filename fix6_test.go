package tomlkit

import "testing"

// N1：map/拷贝构造的内联表分隔
func TestFixN1MapSeparators(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	if err := doc.Set("m", map[string]any{"a": int64(1), "b": int64(2)}); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("N1 map 写回不合法：%v\n%s", perr, out)
	}
	// 拷贝已解析内联表
	doc2 := mustParseOk(t, "src = {x = 1, y = 2}\n")
	if err := doc2.Set("b", doc2.Get("src")); err != nil {
		t.Fatal(err)
	}
	if _, perr := parse([]byte(doc2.String())); perr != nil {
		t.Fatalf("N1 拷贝写回不合法：%s", doc2.String())
	}
}

// N2：含 dotted 成员的内联表无编辑写回保真
func TestFixN2InlineDottedIdentity(t *testing.T) {
	for _, src := range []string{
		"a = {b.c = 1, d = 2}\n",
		"a = {b.c = 1, d = 2, e = 3}\n",
		"a = {b.c = 1, d.e = 2, f = 3}\n",
		"a = {d = 2, b.c = 1}\n",
	} {
		doc := mustParseOk(t, src)
		if got := doc.String(); got != src {
			t.Fatalf("N2 identity：\n%q\nwant %q", got, src)
		}
	}
}

// N3：内联表删除的分隔正确性
func TestFixN3InlineDelete(t *testing.T) {
	doc := mustParseOk(t, "a = {b = 1, c = 2, d = 3}\n")
	if !doc.DeletePath("a.d") {
		t.Fatal("删除末成员")
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("N3 尾逗号：%v\n%s", perr, out)
	}
	doc2 := mustParseOk(t, "a = {b.c = 1, d = 2, e = 3}\n")
	if !doc2.DeletePath("a.b") {
		t.Fatal("删除 dotted 组")
	}
	out2 := doc2.String()
	if _, perr := parse([]byte(out2)); perr != nil {
		t.Fatalf("N3 dotted 删除分隔：%v\n%s", perr, out2)
	}
	// 删 dotted 叶子后空表语义保留
	doc3 := mustParseOk(t, "a = {b.c = 1, d = 2}\n")
	if !doc3.DeletePath("a.b.c") {
		t.Fatal("删除叶子")
	}
	out3 := doc3.String()
	re := mustParseOk(t, out3)
	itb, _ := re.Get("a").InlineTable()
	if itb.Get("b") == nil {
		t.Fatalf("N3 空表消失：%q", out3)
	}
}

// N6：inline 内 dotted 子树的编辑语义
func TestFixN6InlineDottedEdit(t *testing.T) {
	// a) dotted 中间表上 Set 新键
	doc := mustParseOk(t, "a = {b.c = 1}\n")
	itb0, _ := doc.Get("a").InlineTable()
	b, _ := itb0.Get("b").Table()
	if err := b.Set("d", int64(2)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if v, ok := mustParseOk(t, out).GetPathKeys([]string{"a", "b", "d"}).AsInteger(); !ok || v != 2 {
		t.Fatalf("N6a 语义：%q", out)
	}
	// b) SetPath 穿 inline dotted
	if err := doc.SetPath("a.b.e", int64(3)); err != nil {
		t.Fatal(err)
	}
	if _, perr := parse([]byte(doc.String())); perr != nil {
		t.Fatalf("N6b 写回：%v\n%s", perr, doc.String())
	}
	// c) inline 内新建链
	doc2 := mustParseOk(t, "a = {b = 1}\n")
	if err := doc2.SetPath("a.c.d", int64(2)); err != nil {
		t.Fatal(err)
	}
	out2 := doc2.String()
	if _, perr := parse([]byte(out2)); perr != nil {
		t.Fatalf("N6c 写回不合法：%v\n%s", perr, out2)
	}
}

// N4：AoT Append/Set 不打穿后续兄弟表
func TestFixN4AoTSiblings(t *testing.T) {
	doc := mustParseOk(t, "[[x]]\na = 1\n[y]\nb = 2\n")
	arr, _ := doc.Get("x").Array()
	nt := NewTable()
	_ = nt.Set("m1", int64(1))
	_ = nt.Set("m2", int64(2))
	if err := arr.Append(nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatalf("N4 Append 不合法：%v\n%s", perr, out)
	}
	xArr, _ := re.Get("x").Array()
	if xArr.Len() != 2 {
		t.Fatalf("N4 Append 语义：%q", out)
	}
	yb, _ := re.Get("y").Table()
	if v, ok := yb.Get("b").AsInteger(); !ok || v != 2 || len(yb.Keys()) != 1 {
		t.Fatalf("N4 y 被污染：%q", out)
	}
	// Set 增长
	doc2 := mustParseOk(t, "[[x]]\na = 1\n[y]\nb = 2\n")
	arr2, _ := doc2.Get("x").Array()
	big := NewTable()
	for i := 1; i <= 4; i++ {
		_ = big.Set("n"+string(rune('0'+i)), int64(i))
	}
	if err := arr2.Set(0, big); err != nil {
		t.Fatal(err)
	}
	out2 := doc2.String()
	re2, perr2 := parse([]byte(out2))
	if perr2 != nil {
		t.Fatalf("N4 Set 不合法：%v\n%s", perr2, out2)
	}
	yb2, _ := re2.Get("y").Table()
	if len(yb2.Keys()) != 1 || yb2.Keys()[0] != "b" {
		t.Fatalf("N4 Set y 被污染：%q", out2)
	}
}

// N5：无尾换行的 AoT 上 Append/Set
func TestFixN5AoTNoEOL(t *testing.T) {
	doc := mustParseOk(t, "[[x]]")
	arr, _ := doc.Get("x").Array()
	nt := NewTable()
	_ = nt.Set("m", int64(1))
	if err := arr.Append(nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("N5 Append：%v\n%q", perr, out)
	}
	doc2 := mustParseOk(t, "[[x]]")
	arr2, _ := doc2.Get("x").Array()
	nt2 := NewTable()
	_ = nt2.Set("m", int64(1))
	if err := arr2.Set(0, nt2); err != nil {
		t.Fatal(err)
	}
	if _, perr := parse([]byte(doc2.String())); perr != nil {
		t.Fatalf("N5 Set：%v\n%q", perr, doc2.String())
	}
}

// N7：子表追加与祖先晚定义表头
func TestFixN7LateSuperTable(t *testing.T) {
	doc := mustParseOk(t, "[x.y]\na = 1\n[x]\n")
	xy, _ := doc.GetPathKeys([]string{"x", "y"}).Table()
	if err := xy.Set("b", int64(2)); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatalf("N7 不合法：%v\n%s", perr, out)
	}
	if v, ok := re.GetPathKeys([]string{"x", "y", "b"}).AsInteger(); !ok || v != 2 {
		t.Fatalf("N7 语义：%q", out)
	}
}

// N8：表头隐式父表上 Set 子表
func TestFixN8ImplicitPathSeg(t *testing.T) {
	doc := mustParseOk(t, "[t.a]\nx = 1\n")
	tb, _ := doc.Get("t").Table()
	nt := NewTable()
	_ = nt.Set("k", int64(7))
	if err := tb.Set("c", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if v, ok := mustParseOk(t, out).GetPathKeys([]string{"t", "c", "k"}).AsInteger(); !ok || v != 7 {
		t.Fatalf("N8 语义：%q", out)
	}
}

// N9：整表替换内容溢出到交错兄弟
func TestFixN9ReplaceInterleaved(t *testing.T) {
	doc := mustParseOk(t, "[a]\nx = 1\n[b]\ny = 2\n[a.c]\nz = 3\n")
	nt := NewTable()
	for i := 1; i <= 4; i++ {
		_ = nt.Set("n"+string(rune('0'+i)), int64(i))
	}
	if err := doc.Set("a", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatalf("N9 不合法：%v\n%s", perr, out)
	}
	at, _ := re.Get("a").Table()
	if len(at.Keys()) != 4 {
		t.Fatalf("N9 a 键数：%v\n%s", at.Keys(), out)
	}
	bt, _ := re.Get("b").Table()
	if len(bt.Keys()) != 1 || bt.Keys()[0] != "y" {
		t.Fatalf("N9 b 被污染：%v\n%s", bt.Keys(), out)
	}
}
