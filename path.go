package tomlkit

// 点路径糖：字符串点分路径与显式键段两种形态。字符串形态按字面
// '.' 切分，键名本身含点时用 Keys 变体。

import (
	"fmt"
	"strings"
)

// GetPath 按点分路径导航，未命中返回 nil。
func (d *Document) GetPath(path string) Item {
	return d.GetPathKeys(strings.Split(path, "."))
}

// SetPath 按点分路径设置值。中间路径不存在时自动建表（dotted 形态）。
func (d *Document) SetPath(path string, v any) error {
	return d.SetPathKeys(strings.Split(path, "."), v)
}

// DeletePath 按点分路径删除，返回是否删除。
func (d *Document) DeletePath(path string) bool {
	return d.DeletePathKeys(strings.Split(path, "."))
}

// GetPathKeys 按显式键段导航（穿过标准表与内联表）。
func (d *Document) GetPathKeys(keys []string) Item {
	if len(keys) == 0 || d.root == nil {
		return nil
	}
	var cur tableLike = d.root
	for i := 0; i < len(keys)-1; i++ {
		it, ok := cur.get(keys[i])
		if !ok {
			return nil
		}
		switch e := it.(type) {
		case *Table:
			cur = e
		case *InlineTable:
			cur = e
		default:
			return nil
		}
	}
	it, _ := cur.get(keys[len(keys)-1])
	return it
}

// SetPathKeys 按显式键段设置值。
func (d *Document) SetPathKeys(keys []string, v any) error {
	if len(keys) == 0 {
		return fmt.Errorf("tomlkit: empty path")
	}
	if d.root == nil {
		root := &Table{items: map[string]Item{}}
		d.root = root
		root.doc = d
	}
	for _, k := range keys {
		if err := validateKey(k); err != nil {
			return err
		}
	}
	var cur tableLike = d.root
	baseContainer := d.root // 最后真实存在的标准表（新建隐式链的定位基准）
	base := 0               // keyRaw 相对哪个表（最后一个真实存在的表之后的段）
	for i := 0; i < len(keys)-1; i++ {
		name := keys[i]
		existing, ok := cur.get(name)
		if !ok {
			nt := &Table{items: map[string]Item{}, implicit: true, dottedCreated: true, owner: nil}
			if tb, isT := cur.(*Table); isT {
				nt.pathSeg = joinKeyPath(tb, name) // 相对 cur 拼接（cur 链已含前缀）
				nt.owner = tb
			}
			cur.set(name, nt)
			cur = nt
			continue
		}
		switch e := existing.(type) {
		case *Table:
			cur = e
			if e.hasHeader {
				// 有表头的表是独立渲染语境，keyRaw 相对其后
				base = i + 1
			}
			if e.hasHeader {
				baseContainer = e
			}
		case *InlineTable:
			cur = e
			// inline 语境：keyRaw 相对 inline（渲染在 inline 内展开）
			base = i + 1
		default:
			return fmt.Errorf("tomlkit: path segment %q is not a table", name)
		}
	}
	leaf := keys[len(keys)-1]
	if _, exists := cur.get(leaf); exists {
		if itb, ok := cur.(*InlineTable); ok {
			return itb.Set(leaf, v)
		}
		if tb, ok := cur.(*Table); ok {
			return tb.Set(leaf, v)
		}
		return fmt.Errorf("tomlkit: cannot set in this container")
	}
	if itb, ok := cur.(*InlineTable); ok {
		// 内联表内新成员：相对名，渲染走内联表重渲染
		return itb.Set(leaf, v)
	}
	t := cur.(*Table)
	node, err := toNode(v)
	if err != nil {
		return err
	}
	// 新键的 keyRaw 相对目标表：全新路径是全路径 dotted 形态（渲染于
	// root 语境），已存在的表内是相对段（渲染于该表区块内）。
	rel := keys[base:]
	rawSegs := make([]string, len(rel))
	for i, k := range rel {
		rawSegs[i] = quoteKeyIfNeeded(k)
	}
	// 定位基准是最后真实容器：新建的空壳隐式链没有行上下文，
	// appendStyle 会返回 seq=1 与文档首行碰撞（B2）。
	eol, seq := baseContainer.appendStyle()
	setNodeDecor(node, decor{
		keyRaw: strings.Join(rawSegs, "."),
		sep:    " = ",
		eol:    eol,
		seq:    seq,
		dotted: len(rel) > 1,
	})
	_ = rel
	t.set(leaf, node)
	return nil
}

// DeletePathKeys 按显式键段删除（穿过标准表与内联表）。
func (d *Document) DeletePathKeys(keys []string) bool {
	if len(keys) == 0 || d.root == nil {
		return false
	}
	var cur tableLike = d.root
	for i := 0; i < len(keys)-1; i++ {
		it, ok := cur.get(keys[i])
		if !ok {
			return false
		}
		switch e := it.(type) {
		case *Table:
			cur = e
		case *InlineTable:
			cur = e
		default:
			return false
		}
	}
	if tb, ok := cur.(*Table); ok {
		return tb.Delete(keys[len(keys)-1])
	}
	if itb, ok := cur.(*InlineTable); ok {
		return itb.Delete(keys[len(keys)-1])
	}
	return false
}
