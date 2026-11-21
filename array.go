package tomlkit

// InlineTable 是 TOML 内联表 {k = v, ...}。无表头行与区块语义，
// 写回渲染为一对花括号内的成员序列。
type InlineTable struct {
	d     decor
	keys  []string
	items map[string]Item
	// 成员级格式：pre 为 { 后首成员前原文；seps[i] 为成员 i 后（含逗号）
	// 到下一成员（或 }）的原文
	pre  string
	seps []string
}

// Array 是 TOML 数组，含 array of tables（[[x]] 累积出的表数组）。
type Array struct {
	d     decor
	items []Item
	aot   bool   // [[x]] 表数组形态（写回渲染 [[…]] 表头）
	owner *Table // 宿主表（AoT 编辑时平移兄弟区块用）
	// 逐元素的格式信息，与 items 一一对应（渲染按结构逐元素重组）
	elems []arrayElem
	// 末元素之后到闭合括号之间的原文：可选尾逗号、空白、换行
	closeRaw string
}

// arrayElem 是多行数组中一个元素的上下文原文。
type arrayElem struct {
	prefix string // 前一个逗号结束到本元素开始：空白、换行、注释
	tail   string // 元素结束到后随逗号（或闭合括号）的原文
}

// tableLike 是表形容器（Table/InlineTable）的内部统一接口，供 dotted 展开复用。
type tableLike interface {
	set(string, Item)
	get(string) (Item, bool)
}

func (t *InlineTable) set(name string, it Item) {
	if t.items == nil {
		t.items = map[string]Item{}
	}
	if _, ok := t.items[name]; !ok {
		t.keys = append(t.keys, name)
	}
	t.items[name] = it
}

func (t *InlineTable) get(name string) (Item, bool) {
	it, ok := t.items[name]
	return it, ok
}
