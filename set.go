package tomlkit

// Set 语义：命中已有键原位替换（保留缩进与行尾注释），未命中表尾追加。

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Set 设置表中键的值。v 接受 bool、int、int64、float64、string、
// Datetime、[]any、map[string]any、Item。不可渲染的类型返回 error。
func (t *Table) Set(name string, v any) error {
	if err := validateKey(name); err != nil {
		return err
	}
	if tb, ok := v.(*Table); ok {
		return t.setTable(name, tb)
	}
	node, err := toNode(v)
	if err != nil {
		return err
	}
	if old, ok := t.items[name]; ok {
		if isKVForm(old) {
			od, _ := decorOfItem(old)
			setNodeDecor(node, decor{
				prefix: od.prefix, keyRaw: od.keyRaw, sep: od.sep,
				tail: od.tail, eol: od.eol, dotted: od.dotted, seq: od.seq,
			})
			t.set(name, node)
			return nil
		}
		// 区块形态（表/AoT）被标量替换：区块移除，按追加处理
		t.removeItem(name)
	}
	eol, seq := t.appendStyle()
	d := decor{keyRaw: quoteKeyIfNeeded(name), sep: " = ", eol: eol, seq: seq}
	// 边界表头的尾注释段挪到新行之前（对齐 Python：新键落在表尾注释之后）
	if h := nextHeaderEntryAfter(rootOf(t), seq); h != nil {
		if rest, comments := splitTrailingComments(h.d.prefix); comments != "" {
			h.d.prefix = rest
			d.prefix = comments
		}
	}
	setNodeDecor(node, d)
	t.set(name, node)
	return nil
}

// isKVForm 报告条目是否 kv 行形态（继承 decor 原位替换安全）。
func isKVForm(it Item) bool {
	switch e := it.(type) {
	case *Table:
		return false
	case *Array:
		return !e.aot
	}
	return true
}

// tailEntryOf 返回树中最后一个行条目（用于深度 eol 探测与修补）。
func tailEntryOf(it Item) Item {
	switch e := it.(type) {
	case *Table:
		if n := len(e.keys); n > 0 {
			return tailEntryOf(e.items[e.keys[n-1]])
		}
		return e
	case *Array:
		if e.aot && len(e.items) > 0 {
			return tailEntryOf(e.items[len(e.items)-1])
		}
		return e
	}
	return it
}

// deepEolOf 返回条目（含区块）末行的 eol。
func deepEolOf(it Item) string {
	tail := tailEntryOf(it)
	if d, ok := decorOfItem(tail); ok {
		return d.eol
	}
	return ""
}

// appendEOL 返回追加行应使用的换行：文件主导风格（统计多数）。
// 无尾换行的文件先补齐末行（写回合法性优先的最小必要修改）。
func (t *Table) appendEOL() string {
	t.ensureTrailingEOL()
	return t.dominantEOL()
}

// ensureTrailingEOL 在追加前确保树中末行有换行（无尾换行文件必须补，
// 否则追加行粘连；这是对未触及内容的最小必要修改，写回合法性优先）。
func (t *Table) ensureTrailingEOL() string {
	if n := len(t.keys); n > 0 {
		last := t.items[t.keys[n-1]]
		if e := deepEolOf(last); e != "" {
			return e
		}
		dominant := t.dominantEOL()
		setDeepEol(tailEntryOf(last), dominant)
		return dominant
	}
	if t.hasHeader {
		if t.d.eol == "" {
			t.d.eol = t.dominantEOL()
		}
		return t.d.eol
	}
	return "\n"
}

func setDeepEol(it Item, eol string) {
	switch e := it.(type) {
	case *Table:
		e.d.eol = eol
	case *InlineTable:
		e.d.eol = eol
	case *Array:
		e.d.eol = eol
	case *String:
		e.d.eol = eol
	case *Integer:
		e.d.eol = eol
	case *Float:
		e.d.eol = eol
	case *Boolean:
		e.d.eol = eol
	case *Datetime:
		e.d.eol = eol
	}
}

// dominantEOL 统计树中 CRLF 与 LF 出现次数取多数（平局 LF）。
func (t *Table) dominantEOL() string {
	crlf, lf := 0, 0
	count := func(s string) {
		switch s {
		case "\r\n":
			crlf++
		case "\n":
			lf++
		}
	}
	// trivia（prefix/tail/trailing）里的换行也计入主导统计（规范 13）
	countText := func(s string) {
		if s == "" {
			return
		}
		crlf += strings.Count(s, "\r\n")
		lf += strings.Count(s, "\n") - strings.Count(s, "\r\n")
	}
	var walk func(it Item)
	walk = func(it Item) {
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				count(e.d.eol)
				countText(e.d.prefix)
				countText(e.d.tail)
			} else if e == t {
				// root（无表头）：tail 承载文件尾 trivia
				countText(e.d.tail)
			}
			for _, k := range e.keys {
				walk(e.items[k])
			}
		case *Array:
			if e.aot {
				for _, el := range e.items {
					walk(el)
				}
			} else {
				count(e.d.eol)
				countText(e.d.prefix)
			}
		default:
			if d, ok := decorOfItem(e); ok {
				count(d.eol)
				countText(d.prefix)
				countText(d.tail)
			}
		}
	}
	for _, k := range t.keys {
		walk(t.items[k])
	}
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// Set 是根表 Set。
func (d *Document) Set(name string, v any) error {
	if d.root == nil {
		d.root = &Table{items: map[string]Item{}}
	}
	return d.root.Set(name, v)
}

// Set 设置内联表成员。命中原位（成员 keyRaw 与分隔保留），未命中追加到末成员后。
func (t *InlineTable) Set(name string, v any) error {
	if err := validateKey(name); err != nil {
		return err
	}
	node, err := toNode(v)
	if err != nil {
		return err
	}
	if old, ok := t.items[name]; ok {
		od, _ := decorOfItem(old)
		setNodeDecor(node, decor{
			prefix: od.prefix, keyRaw: od.keyRaw, sep: od.sep,
			tail: od.tail, eol: od.eol, seq: od.seq,
		})
		t.set(name, node)
		return nil
	}
	setNodeDecor(node, decor{keyRaw: quoteKeyIfNeeded(name), sep: " = "})
	if n := len(t.keys); n > 0 {
		if len(t.seps) >= n {
			// 有尾 sep：原尾空白并入成员间分隔，新尾为空
			t.seps[n-1] += ", "
			t.seps = append(t.seps, "")
		} else {
			t.seps = append(t.seps, ", ")
		}
	}
	t.set(name, node)
	return nil
}

// Set 替换数组第 index 个元素。容器保持原形态（多行仍是多行），
// 未修改元素按原文保留。
func (a *Array) Set(index int, v any) error {
	if index < 0 || index >= len(a.items) {
		return fmt.Errorf("tomlkit: array index %d out of range [0,%d)", index, len(a.items))
	}
	if a.aot {
		tb, ok := v.(*Table)
		if !ok {
			return fmt.Errorf("tomlkit: array of tables only accepts *Table")
		}
		old, _ := a.items[index].(*Table)
		old.ensureTrailingEOL()
		eol := old.d.eol
		if eol == "" && a.owner != nil {
			eol = a.owner.dominantEOL() // 无尾换行的 AoT：补主导风格（N5）
		}
		if old.d.eol == "" {
			old.d.eol = eol
		}
		tb.d = old.d
		tb.hasHeader = true
		tb.headerIsAoT = true
		newTail := assignTableContent(tb, eol, old.d.seq)
		a.items[index] = tb
		// 平移合并为一次行级重排：宿主全部成员行 + AoT 其余元素行，
		// 阈值取旧区块首（old.d.seq），交错形态（元素行与兄弟行互穿插）
		// 下两套独立 remap 会基线漂移
		a.shiftAoTWithOwner(index, old.d.seq, newTail)
		return nil
	}
	node, err := toNode(v)
	if err != nil {
		return err
	}
	a.items[index] = node
	return nil
}

// appendStyle 返回表内追加 kv 的 eol 与 seq。新 kv 必须落在该表的
// kv 区末尾：最后一个 kv 之后、下一个子表头之前（root 的 kv 渲染在
// 任何表头之后就语义错位，属于父表）。
func (t *Table) appendStyle() (string, float64) {
	eol := t.appendEOL()
	anchor, firstHeader := t.insertAnchor()
	// 全树边界感知：本表 kv 区的物理边界是行流中 scanFrom 之后的
	// 第一个任意表头（不限于本表子树——中间夹的无关表同样是边界）。
	// anchor=0（无锚语境）时以宿主链最近表头为下限，否则祖先自身的
	// 表头会被当成边界，新键落到语境之外。
	scanFrom := anchor
	if scanFrom == 0 {
		for p := t; p != nil; p = p.owner {
			if p.hasHeader {
				scanFrom = p.d.seq
				break
			}
		}
	}
	mergeHeader := func() {
		if fh := nextHeaderAfter(rootOf(t), scanFrom); fh > 0 && (firstHeader == 0 || fh < firstHeader) {
			firstHeader = fh
		}
	}
	mergeHeader()
	if firstHeader > 0 {
		if anchor > 0 && anchor < firstHeader {
			mid := (anchor + firstHeader) / 2
			if mid == anchor || mid == firstHeader {
				// 中点精度耗尽：把 firstHeader 起的行平移腾出整数位
				shift := (anchor + 2) - firstHeader
				shiftTreeFrom(rootOf(t), firstHeader, shift)
				anchor, firstHeader = t.insertAnchor()
				scanFrom = anchor
				if scanFrom == 0 {
					for p := t; p != nil; p = p.owner {
						if p.hasHeader {
							scanFrom = p.d.seq
							break
						}
					}
				}
				mergeHeader()
				mid = (anchor + firstHeader) / 2
			}
			return eol, mid
		}
		// 无本语境 kv：插在行流中首个表头之前
		return eol, firstHeader - 0.5
	}
	if anchor > 0 {
		seq := anchor + 0.5 // 半步：不与任何整数行 seq 平局
		return eol, ensureSeqGap(t, seq)
	}
	if t.hasHeader {
		seq := t.d.seq + 0.5 // 空表：紧跟表头
		return eol, ensureSeqGap(t, seq)
	}
	return eol, ensureSeqGap(t, 1)
}

// ensureSeqGap kv 追加 seq 的边界兜底：向空表逐键追加时行位预留不足
// （追加空表时 need 按空表计），连续半步会追上边界表头。seq 与行流中
// 已有表头平局或越界时平移该表头起的后继行腾位。
func ensureSeqGap(t *Table, seq float64) float64 {
	if h := nextHeaderAfter(rootOf(t), seq); h > 0 && h <= seq {
		shiftTreeFrom(rootOf(t), h, seq-h+2)
	}
	return seq
}

// insertAnchor 返回 kv 追加的锚点（本语境最大 kv 与自身表头的较大者，
// 本语境 = 直接 kv 与 implicit 穿透叶子，不穿入有表头子表）与其后
// 行流中的首个表头（穿透统计，晚定义超表正确）。
func (t *Table) insertAnchor() (anchor, firstHeader float64) {
	lastKV, _ := scanEntrySeqs(t)
	anchor = lastKV
	if t.hasHeader && t.d.seq > anchor {
		anchor = t.d.seq
	}
	var walk func(tb *Table)
	walk = func(tb *Table) {
		for _, k := range tb.keys {
			switch e := tb.items[k].(type) {
			case *Table:
				if e.hasHeader && e.d.seq > anchor {
					if firstHeader == 0 || e.d.seq < firstHeader {
						firstHeader = e.d.seq
					}
				}
				walk(e)
			case *Array:
				if e.aot {
					for _, el := range e.items {
						if ft, ok := el.(*Table); ok {
							if ft.d.seq > anchor {
								if firstHeader == 0 || ft.d.seq < firstHeader {
									firstHeader = ft.d.seq
								}
							}
							walk(ft)
						}
					}
				}
			}
		}
	}
	walk(t)
	return anchor, firstHeader
}

// shiftMembersFrom 把容器中行序 >= from 的成员整体平移 shift。
func (t *Table) shiftMembersFrom(from, shift float64) {
	if shift <= 0 {
		return
	}
	for _, k := range t.keys {
		if entrySeqOfItem(t.items[k]) >= from {
			shiftItemSeqs(t.items[k], shift)
		}
	}
}

// scanEntrySeqs 扫描表的条目（implicit 子表穿透），返回最大 kv 行 seq
// 与最小子表头行 seq。
func scanEntrySeqs(t *Table) (lastKV, firstHeader float64) {
	for _, k := range t.keys {
		it := t.items[k]
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				if firstHeader == 0 || e.d.seq < firstHeader {
					firstHeader = e.d.seq
				}
			} else {
				lk, fh := scanEntrySeqs(e)
				if lk > lastKV {
					lastKV = lk
				}
				if fh > 0 && (firstHeader == 0 || fh < firstHeader) {
					firstHeader = fh
				}
			}
		case *Array:
			if e.aot {
				if ft, ok := e.items[0].(*Table); ok && (firstHeader == 0 || ft.d.seq < firstHeader) {
					firstHeader = ft.d.seq
				}
			} else if e.d.seq > lastKV {
				lastKV = e.d.seq
			}
		default:
			if d, ok := decorOfItem(it); ok && d.seq > lastKV {
				lastKV = d.seq
			}
		}
	}
	return lastKV, firstHeader
}

func maxSeqOfItem(it Item, cur float64) float64 {
	switch e := it.(type) {
	case *Table:
		if e.hasHeader && e.d.seq > cur {
			cur = e.d.seq
		}
		for _, k := range e.keys {
			cur = maxSeqOfItem(e.items[k], cur)
		}
	case *Array:
		if e.d.seq > cur {
			cur = e.d.seq
		}
		for _, el := range e.items {
			cur = maxSeqOfItem(el, cur)
		}
	default:
		if d, ok := decorOfItem(it); ok && d.seq > cur {
			cur = d.seq
		}
	}
	return cur
}

func decorOfItem(it Item) (decor, bool) {
	switch e := it.(type) {
	case *Table:
		return e.d, true
	case *InlineTable:
		return e.d, true
	case *Array:
		return e.d, true
	case *String:
		return e.d, true
	case *Integer:
		return e.d, true
	case *Float:
		return e.d, true
	case *Boolean:
		return e.d, true
	case *Datetime:
		return e.d, true
	}
	return decor{}, false
}

// toNode 把 Go 值转换为树节点（无 decor，由 Set 位置赋予）。
func toNode(v any) (Item, error) {
	switch x := v.(type) {
	case bool:
		return &Boolean{v: x}, nil
	case int:
		return &Integer{v: int64(x)}, nil
	case int64:
		return &Integer{v: x}, nil
	case float64:
		return &Float{v: x}, nil
	case string:
		if !utf8.ValidString(x) {
			return nil, fmt.Errorf("tomlkit: string value contains invalid UTF-8")
		}
		return &String{v: x}, nil
	case Datetime:
		c := x
		c.d, c.raw = decor{}, ""
		return &c, nil
	case *Datetime:
		if x == nil {
			return nil, fmt.Errorf("tomlkit: nil datetime value")
		}
		c := *x
		c.d, c.raw = decor{}, ""
		return &c, nil
	case []any:
		arr := &Array{}
		for _, el := range x {
			n, err := toNode(el)
			if err != nil {
				return nil, err
			}
			arr.items = append(arr.items, n)
			arr.elems = append(arr.elems, arrayElem{prefix: " "})
		}
		if len(arr.items) > 0 {
			arr.elems[0].prefix = ""
		}
		return arr, nil
	case map[string]any:
		it := &InlineTable{items: map[string]Item{}}
		keys := make([]string, 0, len(x))
		for k := range x {
			if err := validateKey(k); err != nil {
				return nil, err
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n, err := toNode(x[k])
			if err != nil {
				return nil, err
			}
			it.set(k, n)
			it.seps = append(it.seps, ", ")
		}
		if len(it.seps) > 0 {
			it.seps[len(it.seps)-1] = ""
		}
		return it, nil
	case Item:
		if isNilItem(x) {
			return nil, fmt.Errorf("tomlkit: nil item value")
		}
		return valueOfItem(x), nil
	}
	return nil, fmt.Errorf("tomlkit: unsupported value type %T", v)
}

// isNilItem 报告 Item 是否为类型化 nil 指针。
func isNilItem(it Item) bool {
	switch e := it.(type) {
	case *Table:
		return e == nil
	case *InlineTable:
		return e == nil
	case *Array:
		return e == nil
	case *String:
		return e == nil
	case *Integer:
		return e == nil
	case *Float:
		return e == nil
	case *Boolean:
		return e == nil
	case *Datetime:
		return e == nil
	}
	return it == nil
}

// validateKey 校验键名是合法 UTF-8（非法键写回会被静默改写）。
func validateKey(name string) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("tomlkit: key contains invalid UTF-8")
	}
	return nil
}

// valueOfItem 深拷贝一个节点的值（剥离格式信息）。
func valueOfItem(it Item) Item {
	switch e := it.(type) {
	case *String:
		return &String{v: e.v}
	case *Integer:
		return &Integer{v: e.v}
	case *Float:
		return &Float{v: e.v}
	case *Boolean:
		return &Boolean{v: e.v}
	case *Datetime:
		return &Datetime{Kind: e.Kind, Time: e.Time}
	case *Array:
		arr := &Array{}
		for _, el := range e.items {
			arr.items = append(arr.items, valueOfItem(el))
		}
		for _, el := range e.elems {
			arr.elems = append(arr.elems, arrayElem{prefix: el.prefix, tail: el.tail})
		}
		return arr
	case *InlineTable:
		out := &InlineTable{items: map[string]Item{}}
		for _, k := range e.keys {
			out.set(k, valueOfItem(e.items[k]))
			out.seps = append(out.seps, ", ")
		}
		if len(out.seps) > 0 {
			out.seps[len(out.seps)-1] = ""
		}
		return out
	case *Table:
		out := &InlineTable{items: map[string]Item{}}
		for _, k := range e.keys {
			out.set(k, valueOfItem(e.items[k]))
			out.seps = append(out.seps, ", ")
		}
		if len(out.seps) > 0 {
			out.seps[len(out.seps)-1] = ""
		}
		return out
	}
	return &String{v: ""}
}

func quoteKeyIfNeeded(name string) string {
	if name == "" {
		return `""`
	}
	for i := 0; i < len(name); i++ {
		if !isBareKeyByte(name[i]) {
			return quoteBasicString(name)
		}
	}
	return name
}

// formatDatetime 按 kind 渲染 datetime。
func formatDatetime(d *Datetime) string {
	switch d.Kind {
	case KindOffsetDatetime:
		return d.Time.Format(time.RFC3339Nano)
	case KindLocalDatetime:
		return d.Time.Format("2006-01-02T15:04:05.999999999")
	case KindLocalDate:
		return d.Time.Format("2006-01-02")
	default:
		return d.Time.Format("15:04:05.999999999")
	}
}

// setTable 整表替换（行为规范 5）：表头行留在原位置（含前导注释），
// 区块内容整体替换；键原先不存在则文件末尾追加新表（规范 6）。
func (t *Table) setTable(name string, nt *Table) error {
	if nt == nil {
		return fmt.Errorf("tomlkit: nil table value")
	}
	if existing, ok := t.items[name]; ok {
		old, isTable := existing.(*Table)
		if isTable && old.hasHeader {
			// 表头原位：内容整体替换。区块尾 = 旧子树最大行序。
			tailEnd := old.d.seq
			nt.d = old.d
			nt.hasHeader = true
			newTail := assignTableContent(nt, old.d.eol, old.d.seq)
			nt.owner = t
			t.set(name, nt)
			t.shiftLinesAfter(tailEnd, newTail, nt)
			return nil
		}
		if arr, isArray := existing.(*Array); isArray && arr.aot {
			// AoT 替换为普通表：首元素表头位置保留
			first, _ := arr.items[0].(*Table)
			tailEnd := first.d.seq
			nt.d = first.d
			nt.hasHeader = true
			newTail := assignTableContent(nt, first.d.eol, first.d.seq)
			nt.owner = t
			t.set(name, nt)
			t.shiftLinesAfter(tailEnd, newTail, nt)
			return nil
		}
		// 非表条目（标量等）被表替换：按末尾新表处理，同时清掉旧条目
		t.removeItem(name)
	}
	// —— 末尾追加新表 ——
	// 行流边界感知定位：子树视角的 maxSeq+1 可能与行流中无关区块的
	// 表头平局，新表内容会溢出进别人的区块（语义破坏）。新表头+内容
	// 占连续整数行位 [start, start+need-1]，后继表头边界不足时先平移腾位。
	root := rootOf(t)
	// 基线取父表自身行位：空壳容器（挂载后再挂子表）没有条目可扫，
	// 没有基线会把新表头排到行流前部、与既有 kv 平局错位（T7）。
	maxSeq := float64(0)
	if t.hasHeader {
		maxSeq = t.d.seq
	}
	for _, k := range t.keys {
		maxSeq = maxSeqOfItem(t.items[k], maxSeq)
	}
	// 无尾换行探测（须在 appendEOL 补换行之前）：原文末行无 eol 时
	// 追加新表只补末行换行、不前置空行（对齐 Python 探针 R4）
	hadNoEOL := len(t.keys) > 0 && deepEolOf(t.items[t.keys[len(t.keys)-1]]) == ""
	eol := t.appendEOL()
	start := math.Floor(maxSeq) + 1
	need := countTableLines(nt)
	if h := nextHeaderAfter(root, maxSeq); h > 0 && start+float64(need)-1 >= h {
		shiftTreeFrom(root, h, start+float64(need)-h)
	}
	// prefix 规则：空文档首表/无尾换行 → 无前置空行；文档尾追加时
	// 文件尾空行区并入 prefix 全部保留；中位落点时复用后继表头
	// prefix 的前导空行区（原文空行成为新表前置分隔，剩余直连）；
	// 其余前置一个空行
	prefix := t.dominantEOL()
	if start <= 1 || hadNoEOL {
		prefix = ""
	}
	if succ := nextHeaderEntryAfter(root, maxSeq); succ != nil {
		// 落点后继表头的前导空行区转移给新表（对齐 Python：新表
		// 消费落点处已有空行，不重复造空行）
		if blanks := leadingBlanks(succ.d.prefix); blanks != "" {
			succ.d.prefix = strings.TrimPrefix(succ.d.prefix, blanks)
			prefix = blanks
		}
	} else if root.doc != nil && isBlankTrail(root.doc.trailing) {
		// 文档尾落点（Python 探针实测）：表头文档整表 append = 尾空行区
		// 转移到表前 + 1 空行（fuzz case1）；纯 kv 文档 = 尾空行区保留
		// 原位即分隔、prefix 0（kv 文档 append 实测）；子表挂载 = 尾空
		// 行区全部转移（EOFTailBlanks max(n,1)，n=0 时维持 1 空行）
		if t != root {
			prefix = root.doc.trailing
			root.doc.trailing = ""
		} else if rootHasHeaderBlock(root) {
			prefix = root.doc.trailing + prefix
			root.doc.trailing = ""
		} else {
			prefix = ""
		}
	}
	nt.d = decor{prefix: prefix, keyRaw: joinKeyPath(t, name), eol: eol, seq: start}
	nt.hasHeader = true
	nt.constructed = true
	nt.owner = t
	assignTableContent(nt, eol, start)
	t.set(name, nt)
	return nil
}

// isBlankTrail 报告文件尾原文是否全为空行。
func isBlankTrail(s string) bool {
	return s != "" && strings.Trim(s, "\n") == ""
}

// leadingBlanks 返回 prefix 的前导空行区（连续 LF，LF 文件语义）。
func leadingBlanks(prefix string) string {
	i := 0
	for i < len(prefix) && prefix[i] == '\n' {
		i++
	}
	return prefix[:i]
}

// ── 行流边界工具 ──────────────────────────────────────────────
// 追加/删除定位必须感知全文档行流：子树视角算出的 seq 可能落进
// 其它表的区块（语义破坏），以下工具统一以全树为扫描范围。

// rootOf 沿宿主链上溯返回根表。
func rootOf(t *Table) *Table {
	for t.owner != nil {
		t = t.owner
	}
	return t
}

// nextHeaderAfter 返回全树行流中 seq 大于 from 的最小表头 seq，无则 0。
func nextHeaderAfter(root *Table, from float64) float64 {
	h := nextHeaderEntryAfter(root, from)
	if h == nil {
		return 0
	}
	return h.d.seq
}

// nextHeaderEntryAfter 返回全树行流中 seq 大于 from 的最小表头所在表。
func nextHeaderEntryAfter(root *Table, from float64) *Table {
	var best *Table
	var walk func(t *Table)
	walk = func(t *Table) {
		if t.hasHeader && t.d.seq > from && (best == nil || t.d.seq < best.d.seq) {
			best = t
		}
		for _, k := range t.keys {
			switch e := t.items[k].(type) {
			case *Table:
				walk(e)
			case *Array:
				if e.aot {
					for _, el := range e.items {
						if ft, ok := el.(*Table); ok {
							walk(ft)
						}
					}
				}
			}
		}
	}
	walk(root)
	return best
}

// subtreeMaxSeq 表子树（穿透全部成员）的最大行 seq。
func subtreeMaxSeq(t *Table) float64 {
	return maxSeqOfItem(t, 0)
}

// firstHeaderOf 表子树中 seq 最小的表头所在表（自身或子孙）。
func firstHeaderOf(t *Table) *Table {
	var best *Table
	var walk func(tb *Table)
	walk = func(tb *Table) {
		if tb.hasHeader && (best == nil || tb.d.seq < best.d.seq) {
			best = tb
		}
		for _, k := range tb.keys {
			if e, ok := tb.items[k].(*Table); ok {
				walk(e)
			}
		}
	}
	walk(t)
	return best
}

// countTableLines 构造表的渲染行数（表头 + kv 行；数组按单行计——
// 构造语境的数组均为单行形态）。
func countTableLines(t *Table) int {
	n := 1
	for _, k := range t.keys {
		switch e := t.items[k].(type) {
		case *Table:
			n += countTableLines(e)
		case *Array:
			if e.aot {
				for _, el := range e.items {
					if ft, ok := el.(*Table); ok {
						n += countTableLines(ft)
					}
				}
			} else {
				n++
			}
		default:
			n++
		}
	}
	return n
}

// shiftTreeFrom 递归平移树中行 seq ≥ from 的条目（表头在 from 之后的
// 整块平移；表头在前但区块内有后续行的递归进内部）。用于追加定位
// 腾出整数行位。
func shiftTreeFrom(t *Table, from, shift float64) {
	if shift <= 0 {
		return
	}
	for _, k := range t.keys {
		it := t.items[k]
		if tb, ok := it.(*Table); ok {
			if tb.hasHeader && tb.d.seq >= from {
				shiftTableSeqs(tb, shift)
				continue
			}
			shiftTreeFrom(tb, from, shift)
			continue
		}
		if entrySeqOfItem(it) >= from {
			shiftItemSeqs(it, shift)
		}
	}
}

// splitTrailingComments 把表头 prefix 尾部的连续注释段剥出：
// prefix = rest + comments + 空行区，返回 rest+空行区 与 注释段。
// 用于 kv 追加时把边界表头的尾注释挪到新行之前（对齐 Python：
// 新键落在表尾注释之后）。
func splitTrailingComments(prefix string) (rest, comments string) {
	if prefix == "" || !strings.Contains(prefix, "#") {
		return prefix, ""
	}
	lines := strings.Split(prefix, "\n")
	i := len(lines)
	for i > 0 && strings.TrimSpace(lines[i-1]) == "" {
		i--
	}
	j := i
	for j > 0 && strings.HasPrefix(strings.TrimSpace(lines[j-1]), "#") {
		j--
	}
	if j == i {
		return prefix, ""
	}
	tail := append(append([]string{}, lines[:j]...), lines[i:]...)
	return strings.Join(tail, "\n"), strings.Join(lines[j:i], "\n") + "\n"
}

func entrySeqOfItem(it Item) float64 {
	switch e := it.(type) {
	case *Table:
		seqs := collectSeqs(e, nil)
		if len(seqs) == 0 {
			return e.d.seq
		}
		m := seqs[0]
		for _, s := range seqs {
			if s < m {
				m = s
			}
		}
		return m
	case *Array:
		if e.aot && len(e.items) > 0 {
			if ft, ok := e.items[0].(*Table); ok {
				return ft.d.seq
			}
		}
		if d, ok := decorOfItem(e); ok {
			return d.seq
		}
	}
	return 0
}

func shiftItemSeqs(it Item, shift float64) {
	switch e := it.(type) {
	case *Table:
		shiftTableSeqs(e, shift)
	case *Array:
		if e.aot {
			for _, el := range e.items {
				if ft, ok := el.(*Table); ok {
					shiftTableSeqs(ft, shift)
				}
			}
		} else {
			e.d.seq += shift
		}
	default:
		if d, ok := decorOfItem(it); ok {
			d.seq += shift
			setNodeDecor(it, d)
		}
	}
}

// assignTableContent 为新表内容生成行 decor。kv 必须整体排在子表头
// 之前（TOML 语义：表内 kv 区在前，否则 kv 归属子表）。
func assignTableContent(t *Table, eol string, startSeq float64) float64 {
	seq := startSeq + 1 // 内容一律从表头行之后排起
	// 第一轮：kv
	for _, k := range t.keys {
		it := t.items[k]
		if _, isTable := it.(*Table); isTable {
			continue
		}
		d, _ := decorOfItem(it)
		d.keyRaw = quoteKeyIfNeeded(k)
		d.sep = " = "
		d.eol = eol
		d.seq = seq
		d.prefix = ""
		setNodeDecor(it, d)
		seq++
	}
	// 第二轮：子表头
	for _, k := range t.keys {
		e, isTable := t.items[k].(*Table)
		if !isTable {
			continue
		}
		e.d = decor{prefix: "\n", keyRaw: joinKeyPath(t, k), eol: eol, seq: seq}
		e.hasHeader = true
		seq = assignTableContent(e, eol, seq)
	}
	return seq
}

// joinKeyPath 拼子表的表头路径：父有表头用其 keyRaw；父是穿透链节点
// 用其 pathSeg（创建时记录）；否则子键。
func joinKeyPath(parent *Table, key string) string {
	prefix := ""
	if parent.hasHeader && parent.d.keyRaw != "" {
		prefix = parent.d.keyRaw
	} else if parent.pathSeg != "" {
		prefix = parent.pathSeg
	}
	k := quoteKeyIfNeeded(key)
	if prefix != "" {
		return prefix + "." + k
	}
	return k
}

// shiftTableSeqs 对区块整体平移 seq（含表头与全部内容）。
func shiftTableSeqs(t *Table, shift float64) {
	if t.hasHeader {
		t.d.seq += shift
	}
	for _, k := range t.keys {
		switch e := t.items[k].(type) {
		case *Table:
			shiftTableSeqs(e, shift)
		case *Array:
			if e.aot {
				for _, el := range e.items {
					if ft, ok := el.(*Table); ok {
						shiftTableSeqs(ft, shift)
					}
				}
			} else {
				e.d.seq += shift
			}
		default:
			d, _ := decorOfItem(e)
			d.seq += shift
			setNodeDecor(e, d)
		}
	}
}

// Len 返回数组元素数。
func (a *Array) Len() int { return len(a.items) }

// Get 返回数组第 index 个元素，越界返回 nil。
func (a *Array) Get(index int) Item {
	if index < 0 || index >= len(a.items) {
		return nil
	}
	return a.items[index]
}

// Append 追加一个元素到数组末尾（保持容器形态）。
// AoT 数组只接受 *Table：新元素继承最后元素的表头原文。
func (a *Array) Append(v any) error {
	if a.aot {
		tb, ok := v.(*Table)
		if !ok {
			return fmt.Errorf("tomlkit: array of tables only accepts *Table")
		}
		if len(a.items) == 0 {
			return fmt.Errorf("tomlkit: cannot append to empty array of tables")
		}
		last, _ := a.items[len(a.items)-1].(*Table)
		last.ensureTrailingEOL() // AoT 块尾无换行时补齐（N5 深层）
		eol := last.d.eol
		if eol == "" && a.owner != nil {
			eol = a.owner.dominantEOL() // 无尾换行的 AoT：补主导风格（N5）
		}
		if last.d.eol == "" {
			last.d.eol = eol
		}
		oldTail := maxSeqOfItem(a, last.d.seq)
		seq := oldTail + 1
		tb.d = decor{prefix: "", keyRaw: last.d.keyRaw, eol: eol, seq: seq}
		tb.hasHeader = true
		tb.headerIsAoT = true
		newTail := assignTableContent(tb, eol, seq)
		a.items = append(a.items, tb)
		a.shiftAoTWithOwner(len(a.items)-1, oldTail, newTail)
		return nil
	}
	node, err := toNode(v)
	if err != nil {
		return err
	}
	if len(a.items) == 0 {
		a.elems = append(a.elems, arrayElem{})
		a.items = append(a.items, node)
		return nil
	}
	a.elems = append(a.elems, arrayElem{prefix: " "})
	a.items = append(a.items, node)
	return nil
}

// Keys 按出现序返回内联表成员键。
func (t *InlineTable) Keys() []string {
	out := make([]string, len(t.keys))
	copy(out, t.keys)
	return out
}

// Get 返回内联表成员，未命中 nil。
func (t *InlineTable) Get(name string) Item { return t.items[name] }

// Delete 删除内联表成员。
func (t *InlineTable) Delete(name string) bool {
	if _, ok := t.items[name]; !ok {
		return false
	}
	idx := -1
	for i, k := range t.keys {
		if k == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	delete(t.items, name)
	t.keys = append(t.keys[:idx], t.keys[idx+1:]...)
	// seps 与逻辑键对齐：删除键后截断为成员间分隔（丢弃尾分隔，
	// 渲染端回退规范化）
	if n := len(t.keys); n == 0 {
		t.seps = nil
	} else if len(t.seps) > n-1 {
		t.seps = t.seps[:n-1]
	}
	return true
}

// shiftAoTWithOwner 对 AoT 元素替换做一次性行级重排：收集宿主容器
// 全部成员的行与 AoT 其余元素的行，> threshold 的按相对序移到 newTail 后。
func (a *Array) shiftAoTWithOwner(selfIdx int, threshold, newTail float64) {
	var seqs []float64
	var collectTbl func(tb *Table)
	collectTbl = func(tb *Table) {
		if tb.hasHeader {
			seqs = append(seqs, tb.d.seq)
		}
		for _, k := range tb.keys {
			switch e := tb.items[k].(type) {
			case *Table:
				collectTbl(e)
			case *Array:
				if e.aot {
					for i, el := range e.items {
						if e == a && i == selfIdx {
							continue
						}
						if ft, ok := el.(*Table); ok {
							collectTbl(ft)
						}
					}
				} else {
					seqs = append(seqs, e.d.seq)
				}
			default:
				if d, ok := decorOfItem(e); ok {
					seqs = append(seqs, d.seq)
				}
			}
		}
	}
	// 沿 owner 链上溯到根，覆盖任意祖先层的后续兄弟
	rootT := a.owner
	for rootT != nil && rootT.owner != nil {
		rootT = rootT.owner
	}
	if rootT != nil {
		collectTbl(rootT)
	} else {
		for i, el := range a.items {
			if i != selfIdx {
				if ft, ok := el.(*Table); ok {
					collectTbl(ft)
				}
			}
		}
	}
	remap := map[float64]float64{}
	for _, s := range seqs {
		if s > threshold {
			remap[s] = 0
		}
	}
	if len(remap) == 0 {
		return
	}
	var newer []float64
	for s := range remap {
		newer = append(newer, s)
	}
	sort.Float64s(newer)
	for i, s := range newer {
		remap[s] = newTail + 1 + float64(i)
	}
	var applyTbl func(tb *Table)
	applyTbl = func(tb *Table) {
		if tb.hasHeader {
			if nv, hit := remap[tb.d.seq]; hit {
				tb.d.seq = nv
			}
		}
		for _, k := range tb.keys {
			switch e := tb.items[k].(type) {
			case *Table:
				applyTbl(e)
			case *Array:
				if e.aot {
					for i, el := range e.items {
						if e == a && i == selfIdx {
							continue
						}
						if ft, ok := el.(*Table); ok {
							applyTbl(ft)
						}
					}
				} else if nv, hit := remap[e.d.seq]; hit {
					e.d.seq = nv
				}
			default:
				if d, ok := decorOfItem(e); ok {
					if nv, hit := remap[d.seq]; hit {
						d.seq = nv
						setNodeDecor(e, d)
					}
				}
			}
		}
	}
	rootT2 := a.owner
	for rootT2 != nil && rootT2.owner != nil {
		rootT2 = rootT2.owner
	}
	if rootT2 != nil {
		applyTbl(rootT2)
	} else {
		for i, el := range a.items {
			if i != selfIdx {
				if ft, ok := el.(*Table); ok {
					applyTbl(ft)
				}
			}
		}
	}
}

// shiftAoTLinesAfter 对 AoT 其余元素做行级重排（同 shiftLinesAfter 语义）。
func (a *Array) shiftAoTLinesAfter(selfIdx int, oldTail, newTail float64) {
	var seqs []float64
	var collect func(it Item)
	collect = func(it Item) {
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				seqs = append(seqs, e.d.seq)
			}
			for _, k := range e.keys {
				collect(e.items[k])
			}
		default:
			if d, ok := decorOfItem(e); ok {
				seqs = append(seqs, d.seq)
			}
		}
	}
	for i, el := range a.items {
		if i != selfIdx {
			collect(el)
		}
	}
	remap := map[float64]float64{}
	for _, s := range seqs {
		if s > oldTail {
			remap[s] = 0
		}
	}
	if len(remap) == 0 {
		return
	}
	var newer []float64
	for s := range remap {
		newer = append(newer, s)
	}
	sort.Float64s(newer)
	for i, s := range newer {
		remap[s] = newTail + 1 + float64(i)
	}
	var apply func(it Item)
	apply = func(it Item) {
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				if nv, hit := remap[e.d.seq]; hit {
					e.d.seq = nv
				}
			}
			for _, k := range e.keys {
				apply(e.items[k])
			}
		default:
			if d, ok := decorOfItem(e); ok {
				if nv, hit := remap[d.seq]; hit {
					d.seq = nv
					setNodeDecor(e, d)
				}
			}
		}
	}
	for i, el := range a.items {
		if i != selfIdx {
			apply(el)
		}
	}
}

// shiftLinesAfter 把容器各成员子树中行序 > oldTail 的行（排除 exclude），
// 按原相对顺序重排到 newTail 之后。行级操作：交错形态（成员行散布）
// 下整树平移会破坏兄弟区间。
func (t *Table) shiftLinesAfter(oldTail, newTail float64, exclude Item) {
	var seqs []float64
	var collect func(it Item)
	collect = func(it Item) {
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				seqs = append(seqs, e.d.seq)
			}
			for _, k := range e.keys {
				collect(e.items[k])
			}
		case *Array:
			if e.aot {
				for _, el := range e.items {
					collect(el)
				}
			} else {
				seqs = append(seqs, e.d.seq)
			}
		default:
			if d, ok := decorOfItem(e); ok {
				seqs = append(seqs, d.seq)
			}
		}
	}
	for _, k := range t.keys {
		if t.items[k] == exclude {
			continue
		}
		collect(t.items[k])
	}
	remap := map[float64]float64{}
	for _, s := range seqs {
		if s > oldTail {
			remap[s] = 0 // 占位，下面按序赋值
		}
	}
	if len(remap) == 0 {
		return
	}
	var newer []float64
	for s := range remap {
		newer = append(newer, s)
	}
	sort.Float64s(newer)
	for i, s := range newer {
		remap[s] = newTail + 1 + float64(i)
	}
	var apply func(it Item)
	apply = func(it Item) {
		switch e := it.(type) {
		case *Table:
			if e.hasHeader {
				if nv, hit := remap[e.d.seq]; hit {
					e.d.seq = nv
				}
			}
			for _, k := range e.keys {
				apply(e.items[k])
			}
		case *Array:
			if e.aot {
				for _, el := range e.items {
					apply(el)
				}
			} else if remap[e.d.seq] != 0 {
				e.d.seq = remap[e.d.seq]
			}
		default:
			if d, ok := decorOfItem(e); ok {
				if nv, hit := remap[d.seq]; hit {
					d.seq = nv
					setNodeDecor(e, d)
				}
			}
		}
	}
	for _, k := range t.keys {
		if t.items[k] != exclude {
			apply(t.items[k])
		}
	}
}

// collectSeqs 收集子树全部行 seq（表头行与 kv 行）。
func collectSeqs(t *Table, out []float64) []float64 {
	if t.hasHeader {
		out = append(out, t.d.seq)
	}
	for _, k := range t.keys {
		switch e := t.items[k].(type) {
		case *Table:
			out = collectSeqs(e, out)
		case *Array:
			if e.aot {
				for _, el := range e.items {
					if ft, ok := el.(*Table); ok {
						out = collectSeqs(ft, out)
					}
				}
			} else {
				out = append(out, e.d.seq)
			}
		default:
			if d, ok := decorOfItem(e); ok {
				out = append(out, d.seq)
			}
		}
	}
	return out
}

// rootHasHeaderBlock 根层是否存在表头区块（表头文档 vs 纯 kv 文档判别）。
func rootHasHeaderBlock(root *Table) bool {
	for _, k := range root.keys {
		if isBlockItem(root.items[k]) {
			return true
		}
	}
	return false
}

// isBlockItem 表/AoT 区块形态条目。
func isBlockItem(it Item) bool {
	switch e := it.(type) {
	case *Table:
		return e.hasHeader
	case *Array:
		return e.aot
	}
	return false
}
