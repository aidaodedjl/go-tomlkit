package tomlkit

// 从零构建文档的构造器（Task 17 补全量测试）。

// NewDocument 返回一个空文档。
func NewDocument() *Document {
	return &Document{root: &Table{items: map[string]Item{}}}
}

// NewTable 返回一个空表。用于 Set 的整表替换或从零构建。
func NewTable() *Table {
	return &Table{items: map[string]Item{}, hasHeader: true}
}

// NewInlineTable 返回一个空内联表。
func NewInlineTable() *InlineTable {
	return &InlineTable{items: map[string]Item{}}
}

// NewArray 返回一个空数组。
func NewArray() *Array {
	return &Array{}
}
