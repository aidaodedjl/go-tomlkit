package tomlkit

// 写回：未修改条目按 decor 与原文区间重组，保证与输入逐字节一致。
// 修改后的条目（raw 为空）走默认风格渲染器（Task 11 接入）。
//
// 行序模型：一个表的输出行流 = 直接 kv 行 + 自身表头行 + 子表行流，
// 全部按 decor.seq（文档行号）排序。这还原「后定义超表」等表头
// 与内容交错的真实时序（如 [x.y.z.w] 出现在 [x] 之前）。

import (
	"sort"
	"strconv"
	"strings"
)

type lineChunk struct {
	seq    float64
	isHead bool // 表头行（seq 平局时优先于 kv——晚定义超表的边界正确性）
	render func(sb *strings.Builder)
}

func (d *Document) String() string {
	if d == nil || d.root == nil {
		return ""
	}
	var sb strings.Builder
	sb.Write(d.bom)
	renderTableInto(&sb, d.root, "")
	sb.WriteString(d.trailing)
	return sb.String()
}

// Bytes 同 String，返回字节切片。
func (d *Document) Bytes() []byte {
	return []byte(d.String())
}

// renderTableInto 渲染一张表的行流。prefix 是当前渲染语境路径：
// 表有表头即独立语境（prefix 重置）；无表头子表穿透，prefix 延展。
// 区块原子化：有表头的子表是一个整体（表头+内容），永不因 seq 数值
// 溢出到兄弟区块（C4/C5 的修复模型）。
func renderTableInto(sb *strings.Builder, t *Table, prefix string) {
	chunks := t.lineChunks(prefix)
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].seq == chunks[j].seq {
			return chunks[i].isHead && !chunks[j].isHead
		}
		return chunks[i].seq < chunks[j].seq
	})
	for _, c := range chunks {
		c.render(sb)
	}
}

func (t *Table) lineChunks(prefix string) []lineChunk {
	var chunks []lineChunk
	if t.hasHeader && !t.suppressHeader() {
		chunks = append(chunks, t.headerChunk())
		prefix = "" // 有表头：独立渲染语境
	}
	chunks = append(chunks, t.memberChunksAll(prefix)...)
	return chunks
}

// suppressHeader 构造来的纯容器表不渲染自身表头（对齐 Python tomlkit
// super table：table() 装的全是子表时不产出 [x] 行，只有空表或含直接
// kv 时才需要表头）。解析来的表头是原文行，永不抑制。
func (t *Table) suppressHeader() bool {
	if !t.constructed || len(t.keys) == 0 {
		return false
	}
	for _, k := range t.keys {
		if _, isTable := t.items[k].(*Table); !isTable {
			return false
		}
	}
	return true
}

// lineChunksWithHeaderPrefix 渲染穿透链中的有表头表：表头路径拼前缀。
// headerChunk 生成表头行 chunk。
func (t *Table) headerChunk() lineChunk {
	hd := t.d
	open, close := "[", "]"
	if t.headerIsAoT {
		open, close = "[[", "]]"
	}
	return lineChunk{seq: hd.seq, isHead: true, render: func(sb *strings.Builder) {
		sb.WriteString(hd.prefix)
		sb.WriteString(open)
		sb.WriteString(hd.keyRaw)
		sb.WriteString(close)
		sb.WriteString(hd.tail)
		sb.WriteString(hd.eol)
	}}
}

func (t *Table) lineChunksWithHeaderPrefix(fullPath string) []lineChunk {
	hd := t.d
	open, close := "[", "]"
	if t.headerIsAoT {
		open, close = "[[", "]]"
	}
	chunks := []lineChunk{{seq: hd.seq, isHead: true, render: func(sb *strings.Builder) {
		sb.WriteString(hd.prefix)
		sb.WriteString(open)
		sb.WriteString(fullPath)
		sb.WriteString(close)
		sb.WriteString(hd.tail)
		sb.WriteString(hd.eol)
	}}}
	chunks = append(chunks, t.memberChunksAll("")...)
	return chunks
}

// memberChunksAll 返回全部成员的行流（不含自身表头）。
func (t *Table) memberChunksAll(prefix string) []lineChunk {
	var chunks []lineChunk
	for _, k := range t.keys {
		chunks = append(chunks, memberChunks(k, t.items[k], prefix)...)
	}
	return chunks
}

// memberChunks 返回成员的行流。
func memberChunks(key string, it Item, prefix string) []lineChunk {
	switch e := it.(type) {
	case *Table:
		if e.hasHeader {
			// 有表头：表头行（keyRaw 创建时已是全路径）+ 内容平铺
			return e.lineChunks("")
		}
		// 无表头：穿透，prefix 延展（段引号化，含点/空格键安全）
		sub := quoteKeyIfNeeded(key)
		if prefix != "" {
			sub = prefix + "." + sub
		}
		return e.lineChunks(sub)
	case *Array:
		if e.aot {
			var out []lineChunk
			for _, el := range e.items {
				t, ok := el.(*Table)
				if !ok {
					continue
				}
				out = append(out, t.lineChunks("")...)
			}
			return out
		}
		return []lineChunk{kvChunk(e.d, e, prefix)}
	case *InlineTable:
		return []lineChunk{kvChunk(e.d, e, prefix)}
	default:
		d, _ := decorOfItem(e)
		return []lineChunk{kvChunk(d, e, prefix)}
	}
}

func kvChunk(d decor, it Item, prefix string) lineChunk {
	return lineChunk{seq: d.seq, render: func(sb *strings.Builder) { renderKVInto(sb, d, it, prefix) }}
}

// renderKVInto 渲染一个键值行：prefix + keyRaw + sep + 值 + tail + eol。
// 值原文存在则直接用（保真），否则按默认风格渲染。
// 挂在无表头表上的后加键（非 dotted 原文）需要语境前缀（C9）。
func renderKVInto(sb *strings.Builder, d decor, it Item, prefix string) {
	sb.WriteString(d.prefix)
	keyRaw := d.keyRaw
	if prefix != "" && !d.dotted {
		keyRaw = prefix + "." + keyRaw
	}
	sb.WriteString(keyRaw)
	sb.WriteString(d.sep)
	sb.WriteString(renderItemValue(it))
	sb.WriteString(d.tail)
	sb.WriteString(d.eol)
}

// renderItemValue 返回条目值的文本：原文或默认渲染。
func renderItemValue(it Item) string {
	switch e := it.(type) {
	case *String:
		if e.raw != "" {
			return e.raw
		}
		return quoteBasicString(e.v)
	case *Integer:
		if e.raw != "" {
			return e.raw
		}
		return strconv.FormatInt(e.v, 10)
	case *Float:
		if e.raw != "" {
			return e.raw
		}
		return formatFloat(e.v)
	case *Boolean:
		if e.raw != "" {
			return e.raw
		}
		return strconv.FormatBool(e.v)
	case *Datetime:
		if e.raw != "" {
			return e.raw
		}
		return formatDatetime(e)
	case *Array:
		return renderArrayValue(e)
	case *InlineTable:
		return renderInlineTableValue(e)
	case *Table:
		return renderInlineTableValue(inlineViewOf(e))
	}
	return ""
}

// inlineViewOf 把标准表包装为内联表视图（数据视角渲染）。
func inlineViewOf(t *Table) *InlineTable {
	it := &InlineTable{items: map[string]Item{}}
	for _, k := range t.keys {
		it.set(k, t.items[k])
	}
	return it
}

// renderArrayValue 渲染修改后的数组：保持原多行形态与元素间格式。
func renderArrayValue(e *Array) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, el := range e.elems {
		sb.WriteString(el.prefix)
		if i < len(e.items) {
			sb.WriteString(renderItemValue(e.items[i]))
		}
		sb.WriteString(el.tail)
		if i < len(e.elems)-1 {
			sb.WriteString(",")
		}
	}
	sb.WriteString(e.closeRaw)
	sb.WriteString("]")
	return sb.String()
}

// renderInlineTableValue 结构化渲染内联表：pre/seps 与逻辑键对齐
// （dotted 组占一个逻辑位），未修改成员保留原文上下文。
func renderInlineTableValue(e *InlineTable) string {
	var sb strings.Builder
	sb.WriteString("{")
	sb.WriteString(e.pre)
	logicIdx := 0
	writeSep := func() {
		if logicIdx > 0 {
			if logicIdx-1 < len(e.seps) && e.seps[logicIdx-1] != "" {
				sb.WriteString(e.seps[logicIdx-1])
			} else {
				sb.WriteString(", ")
			}
		}
	}
	for _, k := range e.keys {
		it := e.items[k]
		if tb, isTable := it.(*Table); isTable && !tb.hasHeader {
			writeSep()
			leaves := flattenDottedLeaves(tb, quoteKeyIfNeeded(k))
			if len(leaves) == 0 {
				sb.WriteString(quoteKeyIfNeeded(k))
				sb.WriteString(" = {}")
				logicIdx++
				continue
			}
			for _, leaf := range leaves {
				if strings.HasPrefix(leaf.keyRaw, "\x00SEP:") {
					sb.WriteString(strings.TrimPrefix(leaf.keyRaw, "\x00SEP:"))
					continue
				}
				sb.WriteString(leaf.keyRaw)
				sb.WriteString(leaf.sep)
				sb.WriteString(renderItemValue(leaf.item))
			}
			logicIdx++
			continue
		}
		writeSep()
		d, _ := decorOfItem(it)
		keyRaw := d.keyRaw
		if keyRaw == "" {
			keyRaw = quoteKeyIfNeeded(k)
		}
		sb.WriteString(keyRaw)
		if d.sep != "" {
			sb.WriteString(d.sep)
		} else {
			sb.WriteString(" = ")
		}
		sb.WriteString(renderItemValue(it))
		logicIdx++
	}
	if n := len(e.seps); n >= len(e.keys) && n > 0 {
		sb.WriteString(e.seps[len(e.keys)-1])
	}
	sb.WriteString("}")
	return sb.String()
}

// flattenDottedLeaves 按插入序展开 dotted 组的叶子。解析来的叶子
// keyRaw 已是全键原文；后加成员拼语境前缀。
func flattenDottedLeaves(t *Table, prefix string) []dottedLeaf {
	var out []dottedLeaf
	var walk func(tb *Table, pfx string)
	walk = func(tb *Table, pfx string) {
		for j, k := range tb.keys {
			if j > 0 && j-1 < len(tb.inlineSeps) && tb.inlineSeps[j-1] != "" {
				out = append(out, dottedLeaf{keyRaw: "\x00SEP:" + tb.inlineSeps[j-1], item: nil})
			} else if j > 0 {
				out = append(out, dottedLeaf{keyRaw: "\x00SEP:, ", item: nil})
			}
			it := tb.items[k]
			if sub, isT := it.(*Table); isT && !sub.hasHeader {
				sub2 := quoteKeyIfNeeded(k)
				if pfx != "" {
					sub2 = pfx + "." + sub2
				}
				before := len(out)
				walk(sub, sub2)
				if len(out) == before {
					// 深层空子组：以 k = {} 形态保留语义
					out = append(out, dottedLeaf{keyRaw: sub2, sep: " = ", item: NewInlineTable()})
				}
				continue
			}
			d, _ := decorOfItem(it)
			keyRaw := d.keyRaw
			if keyRaw == "" {
				keyRaw = quoteKeyIfNeeded(k)
				if pfx != "" {
					keyRaw = pfx + "." + keyRaw
				}
			} else if !d.dotted {
				// 后加成员：拼语境前缀
				kq := quoteKeyIfNeeded(k)
				if pfx != "" {
					keyRaw = pfx + "." + kq
				} else {
					keyRaw = kq
				}
			}
			sep := d.sep
			if sep == "" {
				sep = " = "
			}
			out = append(out, dottedLeaf{keyRaw: keyRaw, sep: sep, item: it})
		}
	}
	walk(t, prefix)
	return out
}

type dottedLeaf struct {
	keyRaw string
	sep    string
	item   Item
}

func quoteBasicString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\t':
			sb.WriteString(`\t`)
		case '\n':
			sb.WriteString(`\n`)
		case '\f':
			sb.WriteString(`\f`)
		case '\r':
			sb.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7F {
				sb.WriteString(`\u`)
				hex := strconv.FormatInt(int64(r), 16)
				for len(hex) < 4 {
					hex = "0" + hex
				}
				sb.WriteString(hex)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func formatFloat(f float64) string {
	if f != f {
		return "nan"
	}
	if f > 1.7976931348623157e+308 {
		return "inf"
	}
	if f < -1.7976931348623157e+308 {
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
