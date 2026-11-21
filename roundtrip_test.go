package tomlkit

// round-trip identity 全量门禁：toml-test valid 全集 + testdata 样本，
// 解析后原样写回必须与输入逐字节一致（行为规范 1）。

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tomltest "github.com/toml-lang/toml-test"
)

func assertIdentity(t *testing.T, name string, data []byte) {
	t.Helper()
	doc, perr := parse(data)
	if perr != nil {
		t.Errorf("[%s] 解析失败：%v", name, perr)
		return
	}
	if got := doc.String(); got != string(data) {
		t.Errorf("[%s] round-trip 不一致：\n--- in:  %q\n--- out: %q", name, data, got)
	}
}

func TestRoundTripTomlTestValid(t *testing.T) {
	embedded := tomltest.EmbeddedTests()
	list, err := tomltest.Runner{Files: embedded, Version: "1.0.0"}.List()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, p := range list {
		if !strings.HasPrefix(p, "valid/") || strings.HasSuffix(p, ".multi") {
			continue
		}
		data, err := fs.ReadFile(embedded, p+".toml")
		if err != nil {
			t.Fatal(err)
		}
		assertIdentity(t, p, data)
		count++
	}
	if count < 185 {
		t.Fatalf("valid 用例数量异常：%d", count)
	}
}

func TestRoundTripTestdata(t *testing.T) {
	files, err := filepath.Glob("testdata/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("testdata 样本不足：%v", files)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		assertIdentity(t, f, data)
	}
}

func readFileFS(fsys fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(fsys, name)
}
