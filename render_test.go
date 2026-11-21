package tomlkit

import (
	"strings"
	"testing"
)

// 规范 1：解析后不做任何修改，写回输出与输入逐字节一致。

func TestRoundTripIdentity(t *testing.T) {
	bomFile := string(bom) + "a = 1\n"
	cases := []string{
		"",
		"\n",
		"a = 1\n",
		"a = 1",
		"\n# comment\na = 1  # tail\n",
		"# only comments\n# second line\n",
		"a = 1\r\nb = 2\r\n",
		bomFile,
		"arr = [\n  1, # c\n  2,\n]\n",
		"arr = [1, 2, 3]\n",
		"ml = \"\"\"\nline1\nline2\n\"\"\"\n",
		"lit = '''\nraw\n'''\n",
		"it = {a = 1, b = {c = 2}}\n",
		"[[aot]]\nx = 1\n[[aot]]\nx = 2\n",
		"a.b.c = 1\na.b.d = 2\n",
		"[t]\nk = 1\n\n[t.sub]\nk2 = 2\n",
		"[a]\n[a.b]\n",
		"empty = []\n",
		"empty_it = {}\n",
		"  indented = 1  # keep\n",
		"dt = 1979-05-27T07:32:00Z\n",
		"[a] # header comment\nx = 1\n",
		"[[a.b]]\nx = 1\n[[a.b]]\nx = 2\n",
		"s = \"esc \\\"\\\\\\t\\n\"\n",
		"mixed = { a = [1, {b = 2}] }\n",
	}
	for _, src := range cases {
		doc, perr := parse([]byte(src))
		if perr != nil {
			t.Errorf("解析失败 %q: %v", src, perr)
			continue
		}
		if got := doc.String(); got != src {
			t.Errorf("round-trip 不一致：\n--- in:\n%q\n--- out:\n%q", src, got)
		}
		if got := string(doc.Bytes()); got != src {
			t.Errorf("Bytes 不一致：%q", got)
		}
	}
}

// 真实形态样本（两个消费方 config.toml 的结构形态，脱敏）
func TestRoundTripConsumerShape(t *testing.T) {
	src := `# 全局配置
play_audio = true  # 播放提示音

[actions]
# 预留注释行

[actions.wave]
name = "wave"
volume = 0.8
tags = ["a", "b"]

[actions.jump]
name = "jump"
enabled = false

[[actions.seq]]
step = 1
[[actions.seq]]
step = 2
`
	doc, perr := parse([]byte(src))
	if perr != nil {
		t.Fatal(perr)
	}
	if got := doc.String(); got != src {
		t.Errorf("消费方形态 round-trip 不一致：\n%q\n---\n%q", src, got)
	}
	if !strings.Contains(doc.String(), "[actions.wave]") {
		t.Fatal("结构丢失")
	}
}
