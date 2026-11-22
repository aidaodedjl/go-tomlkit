package tomlkit

import "testing"

// T7：挂载后再挂子表——空壳容器没有条目可扫，maxSeq 缺基线会把
// 新表头排到行流前部、与既有 kv 平局错位（version 被吞进新区块）。
func TestFixT7MountThenAttach(t *testing.T) {
	doc := mustParseOk(t, "version = \"1.0\"\n")
	wrap := NewTable()
	if err := doc.Set("actions", wrap); err != nil {
		t.Fatal(err)
	}
	sub := NewTable()
	_ = sub.Set("description", "x")
	if err := wrap.Set("new_dance", sub); err != nil {
		t.Fatal(err)
	}
	want := "version = \"1.0\"\n\n[actions.new_dance]\ndescription = \"x\"\n"
	if got := doc.String(); got != want {
		t.Fatalf("T7 got:\n%q\nwant:\n%q", got, want)
	}
}

// T7b：解析态空表追加子表同样取表头行位为基线，落在自身区块内。
func TestFixT7bParsedEmptyTableSubAppend(t *testing.T) {
	doc := mustParseOk(t, "pre = 0\n\n[a]\n\n[b]\nq = 1\n")
	a, _ := doc.Get("a").Table()
	sub := NewTable()
	_ = sub.Set("x", int64(1))
	if err := a.Set("c", sub); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if _, perr := parse([]byte(out)); perr != nil {
		t.Fatalf("T7b 写回不合法：%v\n%s", perr, out)
	}
	if i, j := indexOf(out, "[a.c]"), indexOf(out, "pre = 0"); j < 0 || i < j {
		t.Fatalf("T7b [a.c] 不得排到既有 kv 之前：\n%s", out)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
