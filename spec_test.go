package tomlkit

import (
	"strings"
	"testing"
)

// spec 行为规范 1-14 逐条对照（编号即用例名）。已由前面任务覆盖的
// 行为在此以规范编号收口并查漏。

func TestSpec1RoundTripIdentity(t *testing.T) {
	src := "# c\na = 1\n\n[t]\nb = \"x\" # tail\n"
	doc := mustParseOk(t, src)
	if doc.String() != src {
		t.Fatalf("规范 1 失败")
	}
}

func TestSpec2InPlaceReplace(t *testing.T) {
	doc := mustParseOk(t, "  a = 1  # note\n")
	_ = doc.Set("a", false)
	if got, want := doc.String(), "  a = false  # note\n"; got != want {
		t.Fatalf("规范 2 失败：%q", got)
	}
}

func TestSpec3DeleteRemovesLeadingComment(t *testing.T) {
	doc := mustParseOk(t, "# lead\ndrop = 1\nkeep = 2\n")
	doc.Delete("drop")
	if got, want := doc.String(), "keep = 2\n"; got != want {
		t.Fatalf("规范 3 失败：%q", got)
	}
}

func TestSpec4AppendAtTableEnd(t *testing.T) {
	doc := mustParseOk(t, "[t]\na = 1\n[u]\nb = 2\n")
	_ = doc.SetPath("t.c", int64(3))
	if !strings.HasPrefix(doc.String(), "[t]\na = 1\nc = 3\n") {
		t.Fatalf("规范 4 失败：%q", doc.String())
	}
}

func TestSpec5TableReplaceKeepsHeader(t *testing.T) {
	doc := mustParseOk(t, "# h\n[a]\nold = 1\n[b]\nc = 2\n")
	nt := NewTable()
	_ = nt.Set("n", int64(1))
	_ = doc.Set("a", nt)
	if !strings.HasPrefix(doc.String(), "# h\n[a]\nn = 1\n") {
		t.Fatalf("规范 5 失败：%q", doc.String())
	}
}

func TestSpec6NewTableBlankLine(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n")
	nt := NewTable()
	_ = nt.Set("x", int64(1))
	_ = doc.Set("new", nt)
	if got, want := doc.String(), "a = 1\n\n[new]\nx = 1\n"; got != want {
		t.Fatalf("规范 6 失败：%q", got)
	}
}

func TestSpec7KeyOrder(t *testing.T) {
	doc := mustParseOk(t, "z = 1\n[a]\nm = 1\nb = 2\n")
	if k := doc.Keys(); k[0] != "z" || k[1] != "a" {
		t.Fatalf("规范 7 失败：%v", k)
	}
	tb, _ := doc.Get("a").Table()
	if k := tb.Keys(); k[0] != "m" || k[1] != "b" {
		t.Fatalf("规范 7 表内失败：%v", k)
	}
}

func TestSpec8DefaultRender(t *testing.T) {
	doc := NewDocument()
	_ = doc.Set("s", "v")
	_ = doc.Set("i", int64(-3))
	_ = doc.Set("f", 2.0)
	_ = doc.Set("b", true)
	out := doc.String()
	for _, want := range []string{`s = "v"`, "i = -3", "f = 2.0", "b = true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("规范 8 失败：%q 缺 %q", out, want)
		}
	}
}

func TestSpec9ArrayShapePreserved(t *testing.T) {
	doc := mustParseOk(t, "a = [\n  1,\n  2,\n]\n")
	arr, _ := doc.Get("a").Array()
	_ = arr.Set(0, "x")
	if !strings.Contains(doc.String(), "a = [\n  \"x\",\n  2,\n]") {
		t.Fatalf("规范 9 失败：%q", doc.String())
	}
}

func TestSpec10DottedKey(t *testing.T) {
	doc := mustParseOk(t, "a.b = 1\n")
	if got, want := doc.String(), "a.b = 1\n"; got != want {
		t.Fatalf("规范 10 保真失败：%q", got)
	}
	_ = doc.SetPath("a.c", int64(2))
	if got, want := doc.String(), "a.b = 1\na.c = 2\n"; got != want {
		t.Fatalf("规范 10 修改失败：%q", got)
	}
	// dotted 与表头冲突在解析期报错
	if _, perr := parse([]byte("a.b = 1\n[a]\n")); perr == nil {
		t.Fatal("规范 10 冲突未报错")
	}
}

func TestSpec11ErrorPosition(t *testing.T) {
	_, perr := parse([]byte("a = 1\nbad =\n"))
	if perr == nil || perr.Line != 2 || perr.Col != 6 {
		t.Fatalf("规范 11 失败：%+v", perr)
	}
	if !strings.Contains(perr.Error(), "line 2") {
		t.Fatalf("规范 11 消息失败：%v", perr)
	}
}

func TestSpec12BOM(t *testing.T) {
	src := string(bom) + "a = 1\n"
	doc := mustParseOk(t, src)
	if doc.String() != src {
		t.Fatalf("规范 12 透传失败")
	}
	bad := append([]byte("a = 1\n"), bom...)
	if _, perr := parse(bad); perr == nil {
		t.Fatal("规范 12 非开头 BOM 未报错")
	}
}

func TestSpec13EOLFollowsFile(t *testing.T) {
	doc := mustParseOk(t, "a = 1\r\n")
	_ = doc.Set("b", int64(2))
	if got, want := doc.String(), "a = 1\r\nb = 2\r\n"; got != want {
		t.Fatalf("规范 13 失败：%q", got)
	}
}

func TestSpec14AlwaysValidOutput(t *testing.T) {
	doc := mustParseOk(t, "a = 1\n\n[t]\nb = [1, 2]\nc = {x = 1}\n")
	_ = doc.Set("a", "改後")
	_ = doc.SetPath("t.d", int64(9))
	arr, _ := doc.GetPath("t.b").Array()
	_ = arr.Set(1, false)
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("规范 14 失败（写回不合法）：%v\n%s", perr, out)
	}
	re := mustParseOk(t, out)
	if v, ok := re.GetPath("t.d").AsInteger(); !ok || v != 9 {
		t.Fatalf("规范 14 语义失败")
	}
}
