package tomlkit

// 编辑行为对拍测试：期望输出由 Python tomlkit 0.14 实际执行同一编辑序列
// 生成后固化（逐字节锁定两库对齐）。edit11 为渲染习惯分歧场景（本库输出
// 锁定，与 tomlkit 的差异已记入 README 已知限制）。
// 语料来源：tomlkit 仓库 tests/examples（MIT），见 testdata/xref/input。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type xrefCase struct {
	name string
	in   string
	ops  []string
}

var xrefCases = []xrefCase{
	{"edit01_scalar", "example.toml", []string{"set", "owner.name", "New Name"}},
	{"edit02_array_to_int", "example.toml", []string{"set", "database.ports", "1"}},
	{"edit03_title", "example.toml", []string{"set", "title", "TOML Example 2"}},
	{"edit04_delete", "example.toml", []string{"del", "owner.dob"}},
	{"edit05_newkey", "example.toml", []string{"set", "new_key", "fresh"}},
	{"edit06_deep_ip", "example.toml", []string{"set", "servers.alpha.ip", "10.0.0.1"}},
	{"edit07_table_replace", "example.toml", []string{"settable", "owner", "name", "X", "y", "2"}},
	{"edit08_db_max", "example.toml", []string{"set", "database.connection_max", "5000"}},
	{"edit09_pyproject_ver", "pyproject.toml", []string{"set", "tool.poetry.version", "2.0.0"}},
	{"edit10_pyproject_dep", "pyproject.toml", []string{"set", "tool.poetry.dependencies.python", "^3.13"}},
	{"edit11_aot_newpath", "fruit.toml", []string{"set", "fruit.apple.color", "green"}},
}

// applyXrefOps 按对拍 DSL 应用编辑（与生成期望时的引擎语义一致）。
func applyXrefOps(t *testing.T, doc *Document, ops []string) {
	t.Helper()
	for i := 0; i < len(ops); i++ {
		switch ops[i] {
		case "set":
			var v any = ops[i+2]
			if ops[i+2] == "true" || ops[i+2] == "false" {
				v = ops[i+2] == "true"
			} else if _, ok := parseInt64(ops[i+2]); ok {
				v = mustInt(ops[i+2])
			}
			if err := doc.SetPath(ops[i+1], v); err != nil {
				t.Fatalf("set %s: %v", ops[i+1], err)
			}
			i += 2
		case "del":
			doc.DeletePath(ops[i+1])
			i++
		case "settable":
			tb := NewTable()
			j := i + 2
			for j+1 < len(ops)+1 && j+1 <= len(ops)-1 {
				if ops[j] == "set" || ops[j] == "del" || ops[j] == "settable" {
					break
				}
				var v any = ops[j+1]
				if _, ok := parseInt64(ops[j+1]); ok {
					v = mustInt(ops[j+1])
				}
				if err := tb.Set(ops[j], v); err != nil {
					t.Fatal(err)
				}
				j += 2
			}
			if err := doc.Set(ops[i+1], tb); err != nil {
				t.Fatalf("settable %s: %v", ops[i+1], err)
			}
			i = j - 1
		}
	}
}

func mustInt(s string) int64 {
	v, _ := parseInt64(s)
	return v
}

func TestXrefEditBehavior(t *testing.T) {
	for _, c := range xrefCases {
		t.Run(c.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata/xref/input", c.in))
			if err != nil {
				t.Fatal(err)
			}
			wantB, err := os.ReadFile(filepath.Join("testdata/xref/expected", c.name+".toml"))
			if err != nil {
				t.Fatal(err)
			}
			doc, perr := Parse(data)
			if perr != nil {
				t.Fatalf("解析：%v", perr)
			}
			applyXrefOps(t, doc, c.ops)
			got := doc.String()
			if got != string(wantB) {
				t.Fatalf("与 tomlkit 输出不一致：\n--- got:\n%s\n--- want(tomlkit):\n%s", got, wantB)
			}
			// 双保险：期望输出本身可重解析
			if _, perr := Parse(wantB); perr != nil {
				t.Fatalf("期望输出不可解析：%v", perr)
			}
		})
	}
}

// TestXrefSourceIdentity：对拍语料原文的 round-trip identity。
func TestXrefSourceIdentity(t *testing.T) {
	entries, _ := os.ReadDir("testdata/xref/input")
	if len(entries) < 3 {
		t.Fatalf("语料缺失：%d", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join("testdata/xref/input", e.Name()))
		assertIdentity(t, "xref/"+e.Name(), data)
	}
}
