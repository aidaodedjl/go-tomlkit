package tomlkit

import "testing"

func TestItemAccessors(t *testing.T) {
	doc := mustParseOk(t, `
b = true
i = 42
f = 3.5
s = "x"
dt = 1979-05-27T07:32:00Z
`)
	cases := []struct {
		key  string
		want ItemType
	}{
		{"b", ItemBoolean}, {"i", ItemInteger}, {"f", ItemFloat},
		{"s", ItemString}, {"dt", ItemDatetime},
	}
	for _, c := range cases {
		it := doc.root.items[c.key]
		if it.Type() != c.want {
			t.Errorf("%s 类型不符：%v", c.key, it.Type())
		}
	}
	if v, ok := doc.root.items["b"].AsBool(); !ok || v != true {
		t.Error("AsBool")
	}
	if _, ok := doc.root.items["i"].AsBool(); ok {
		t.Error("Integer.AsBool 应不命中")
	}
	if v, ok := doc.root.items["i"].AsInteger(); !ok || v != 42 {
		t.Error("AsInteger")
	}
	if v, ok := doc.root.items["f"].AsFloat(); !ok || v != 3.5 {
		t.Error("AsFloat")
	}
	if v, ok := doc.root.items["s"].AsString(); !ok || v != "x" {
		t.Error("AsString")
	}
	if _, ok := doc.root.items["s"].AsInteger(); ok {
		t.Error("String.AsInteger 应不命中")
	}
	dt, ok := doc.root.items["dt"].AsDatetime()
	if !ok || dt.Kind != KindOffsetDatetime {
		t.Error("AsDatetime")
	}
}

func TestItemNavigation(t *testing.T) {
	doc := mustParseOk(t, "[t]\nk = 1\nit = {a = 1}\narr = [1]\n")
	tbl := doc.root.items["t"]
	if tb, ok := tbl.Table(); !ok || tb == nil {
		t.Error("Table()")
	}
	if _, ok := tbl.Array(); ok {
		t.Error("Table.Array 应不命中")
	}
	it := doc.root.items["t"].(*Table).items["it"]
	if itb, ok := it.InlineTable(); !ok || itb == nil {
		t.Error("InlineTable()")
	}
	arr := doc.root.items["t"].(*Table).items["arr"]
	if a, ok := arr.Array(); !ok || a == nil {
		t.Error("Array()")
	}
}

func TestItemAny(t *testing.T) {
	doc := mustParseOk(t, `
top = "level"
num = [1, 2.5, true]
dt = 1979-05-27

[sect]
name = "s"
inner = {x = 1}

[[list]]
v = 1

[[list]]
v = 2
`)
	m, ok := doc.root.Any().(map[string]any)
	if !ok {
		t.Fatal("根 Any 应为 map")
	}
	if m["top"] != "level" {
		t.Error("top")
	}
	arr, ok := m["num"].([]any)
	if !ok || len(arr) != 3 || arr[0] != int64(1) || arr[1] != 2.5 || arr[2] != true {
		t.Errorf("num：%v", m["num"])
	}
	if d, ok := m["dt"].(Datetime); !ok || d.Kind != KindLocalDate {
		t.Error("dt")
	}
	sect, ok := m["sect"].(map[string]any)
	if !ok || sect["name"] != "s" {
		t.Error("sect")
	}
	if _, ok := sect["inner"].(map[string]any); !ok {
		t.Error("inner")
	}
	lst, ok := m["list"].([]any)
	if !ok || len(lst) != 2 {
		t.Errorf("list：%v", m["list"])
	}
}
