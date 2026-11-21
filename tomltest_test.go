package tomlkit

// toml-test 官方套件门禁：valid 全集解析等价（导出 JSON 语义对比），
// invalid 全集拒绝。本库不做 JSON→TOML 编码（String() 是保真写回，
// 不是序列化器），Encode 返回 unsupported。

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	tomltest "github.com/toml-lang/toml-test"
)

type ttTag struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func exportTomlTestValue(it Item) any {
	switch e := it.(type) {
	case *String:
		return ttTag{"string", e.v}
	case *Integer:
		return ttTag{"integer", strconv.FormatInt(e.v, 10)}
	case *Float:
		return ttTag{"float", ttFloat(e.v)}
	case *Boolean:
		return ttTag{"bool", strconv.FormatBool(e.v)}
	case *Datetime:
		name, val := ttDatetime(e)
		return ttTag{name, val}
	case *Array:
		arr := make([]any, len(e.items))
		for i, v := range e.items {
			arr[i] = exportTomlTestValue(v)
		}
		return arr
	case *Table:
		m := make(map[string]any, len(e.items))
		for _, k := range e.keys {
			m[k] = exportTomlTestValue(e.items[k])
		}
		return m
	case *InlineTable:
		m := make(map[string]any, len(e.items))
		for _, k := range e.keys {
			m[k] = exportTomlTestValue(e.items[k])
		}
		return m
	}
	return nil
}

func ttFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "nan"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func ttDatetime(d *Datetime) (string, string) {
	switch d.Kind {
	case KindOffsetDatetime:
		return "datetime", d.Time.Format(time.RFC3339Nano)
	case KindLocalDatetime:
		return "datetime-local", d.Time.Format("2006-01-02T15:04:05.999999999")
	case KindLocalDate:
		return "date-local", d.Time.Format("2006-01-02")
	default:
		return "time-local", d.Time.Format("15:04:05.999999999")
	}
}

type tomlkitParser struct{}

func (tomlkitParser) Encode(ctx context.Context, jsonInput string) (string, bool, error) {
	return "encoding from JSON is not supported by tomlkit", true, nil
}

func (tomlkitParser) Decode(ctx context.Context, tomlInput string) (string, bool, error) {
	doc, err := Parse([]byte(tomlInput))
	if err != nil {
		return err.Error(), true, nil
	}
	b, mErr := json.Marshal(exportTomlTestValue(doc.root))
	if mErr != nil {
		return "", false, mErr
	}
	return string(b), false, nil
}

func TestTomlTest(t *testing.T) {
	rs, err := tomltest.Runner{
		Files:   tomltest.EmbeddedTests(),
		Parser:  tomlkitParser{},
		Version: "1.0.0",
	}.Run()
	if err != nil {
		t.Fatal(err)
	}
	var fails int
	for _, tt := range rs.Tests {
		if tt.Failed() {
			fails++
			t.Errorf("[%s]\n--- input:\n%s\n--- want:\n%s\n--- got:\n%s\n--- failure: %s",
				tt.Path, tt.Input, tt.Want, tt.Output, tt.Failure)
		}
	}
	if rs.PassedValid < 185 || rs.PassedInvalid < 371 {
		t.Fatalf("toml-test 用例数量异常：%d valid / %d invalid（期望 ≥185/≥371）",
			rs.PassedValid, rs.PassedInvalid)
	}
	if rs.Skipped != 0 {
		t.Fatalf("toml-test 跳过了 %d 个用例（承诺 0 skip）", rs.Skipped)
	}
	if fails > 0 {
		t.Fatalf("toml-test: %d 个用例失败", fails)
	}
	t.Logf("toml-test: %d valid + %d invalid, 0 skipped", rs.PassedValid, rs.PassedInvalid)
}
