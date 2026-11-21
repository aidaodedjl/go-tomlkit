package tomlkit

// Go 原生 fuzz：核心不变量是「解析成功 → 无修改写回逐字节一致」与
// 「任何输入不 panic」。种子取自 toml-test 抽样与 testdata。

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tomltest "github.com/toml-lang/toml-test"
)

func FuzzParseRoundTrip(f *testing.F) {
	// testdata 语料
	entries, _ := filepath.Glob("testdata/*.toml")
	for _, e := range entries {
		if b, err := os.ReadFile(e); err == nil {
			f.Add(b)
		}
	}
	// 对拍语料
	xref, _ := filepath.Glob("testdata/xref/input/*.toml")
	for _, e := range xref {
		if b, err := os.ReadFile(e); err == nil {
			f.Add(b)
		}
	}
	// toml-test valid 抽样（每类一个）
	embedded := tomltest.EmbeddedTests()
	list, err := tomltest.Runner{Files: embedded, Version: "1.0.0"}.List()
	if err == nil {
		seen := map[string]bool{}
		for _, p := range list {
			if !strings.HasPrefix(p, "valid/") || strings.HasSuffix(p, ".multi") {
				continue
			}
			cat := strings.SplitN(strings.TrimPrefix(p, "valid/"), "/", 2)[0]
			if seen[cat] {
				continue
			}
			seen[cat] = true
			if b, err := fsRead(embedded, p+".toml"); err == nil {
				f.Add(b)
			}
		}
	}
	// 手工边界种子
	f.Add([]byte("a = [1,\r2]"))
	f.Add([]byte("[[x]]"))
	f.Add([]byte("\xEF\xBB\xBFa = 1\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, perr := Parse(data)
		if perr != nil {
			return // 拒绝即可，不 panic
		}
		out := doc.String()
		if out != string(data) {
			t.Errorf("identity 失败：\n--- in:\n%q\n--- out:\n%q", data, out)
		}
		// 重解析语义等价
		re, perr2 := Parse([]byte(out))
		if perr2 != nil {
			t.Errorf("写回不可解析：%v\n%q", perr2, out)
			return
		}
		if len(doc.Keys()) != len(re.Keys()) {
			t.Errorf("根键数漂移：%d vs %d\n%q", len(doc.Keys()), len(re.Keys()), out)
		}
	})
}

func fsRead(fsys fs.FS, name string) ([]byte, error) { return fs.ReadFile(fsys, name) }
