package tomlkit

import "strings"

// Table 是 TOML 表。标准表（[x] 形态）与内联表共用此结构，
// InlineTable 是它的类型别名约束（见 item.go）。
type Table struct {
	d             decor
	keys          []string        // 出现序
	items         map[string]Item // 键为解码后的字符串
	implicit      bool            // dotted key 中间节点或被 [a.b] 隐式创建
	dottedCreated bool            // 由 dotted key 创建（区别于 [a.b] 隐式中间表）
	hasHeader     bool            // 有显式 [x] 表头行
	headerIsAoT   bool            // [[x]] 表数组形态（渲染双方括号）
	pathSeg       string          // 穿透链路径段（无表头节点上新建子表时拼表头用）
	owner         *Table          // 宿主表（嵌套区块行平移时上溯用；root/独立表为 nil）
	inlineSeps    []string        // inline 语境下本表成员间的原文分隔（dotted 组内保真）
	constructed   bool            // 由 Set 追加的构造表（纯容器形态不渲染自身表头，对齐 Python super table）
	doc           *Document       // 根表上的文档反指针（尾空行区转移用）
}

// set 插入或覆盖一个键，维护出现序。
func (t *Table) set(name string, it Item) {
	if t.items == nil {
		t.items = map[string]Item{}
	}
	if _, ok := t.items[name]; !ok {
		t.keys = append(t.keys, name)
	}
	t.items[name] = it
}

func (t *Table) get(name string) (Item, bool) {
	it, ok := t.items[name]
	return it, ok
}

// Get 返回表中键的值，未命中返回 nil。
func (t *Table) Get(name string) Item {
	return t.items[name]
}

// Keys 按文件出现序返回表的键（含子表与 dotted 中间表）。
func (t *Table) Keys() []string {
	out := make([]string, len(t.keys))
	copy(out, t.keys)
	return out
}

// Get 是根表 Get。
func (d *Document) Get(name string) Item {
	if d.root == nil {
		return nil
	}
	return d.root.Get(name)
}

// Keys 是根表 Keys。
func (d *Document) Keys() []string {
	if d.root == nil {
		return nil
	}
	return d.root.Keys()
}

// Delete 删除键。kv 条目连同其前导注释（decor.prefix）一起移除；
// 表条目删整个区块，区块 trivia 按 Python tomlkit 探针规则处理
// （M3：被删首表头的 prefix 转移给行流后继表头——块前注释保留；
// 被删 prefix 为空时剥后继表头 prefix 的首个空行——块尾空行随块删）。
// 键不存在返回 false。
func (t *Table) Delete(name string) bool {
	if _, ok := t.items[name]; !ok {
		return false
	}
	if old, isTable := t.items[name].(*Table); isTable {
		transferHeaderTrivia(rootOf(t), old)
	}
	t.removeItem(name)
	return true
}

// removeItem 仅移除条目（不做 trivia 转移，Set 的替换路径用）。
func (t *Table) removeItem(name string) {
	delete(t.items, name)
	for i, k := range t.keys {
		if k == name {
			t.keys = append(t.keys[:i], t.keys[i+1:]...)
			break
		}
	}
}

// transferHeaderTrivia 删除表区块时的 trivia 归位（root 为全树根）。
func transferHeaderTrivia(root, old *Table) {
	succ := nextHeaderEntryAfter(root, subtreeMaxSeq(old))
	if succ == nil {
		// 文档尾区块：块尾空行区随块删（对齐 Python：删除文档尾表后
		// append 的空行分布——fuzz case3）；尾注释保留
		if root.doc != nil && isBlankTrail(root.doc.trailing) {
			root.doc.trailing = ""
		}
		return
	}
	first := firstHeaderOf(old)
	victim := ""
	if first != nil {
		victim = first.d.prefix
	}
	if victim != "" {
		succ.d.prefix = victim
	} else if strings.HasPrefix(succ.d.prefix, "\n") {
		succ.d.prefix = strings.TrimPrefix(succ.d.prefix, "\n")
	}
}

// Delete 是根表 Delete。
func (d *Document) Delete(name string) bool {
	if d.root == nil {
		return false
	}
	return d.root.Delete(name)
}
