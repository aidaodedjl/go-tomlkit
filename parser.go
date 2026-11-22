package tomlkit

// parser 把 token 流组装成文档树。decor 从相邻 token 的原文区间重组：
// 每个字节都属于某个 token，相邻 token 的区间差值即格式原文。

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

func parse(data []byte) (*Document, *ParseError) {
	toks, perr := lex(data)
	if perr != nil {
		return nil, perr
	}
	root := &Table{items: map[string]Item{}}
	doc := &Document{root: root}
	root.doc = doc
	p := &parser{data: data, toks: toks, doc: doc}
	return p.run()
}

type parser struct {
	data    []byte
	toks    []token
	doc     *Document
	cur     *Table // 当前表
	prevEnd int    // 上一实质行结束（eol 之后）的字节位置
}

func (p *parser) failAt(tk *token, msg string) *ParseError {
	return &ParseError{Line: tk.line, Col: tk.col, Offset: tk.off, Snippet: p.lineSnippet(tk.line), Msg: msg}
}

func (p *parser) lineSnippet(line int) string {
	// 从 data 提取指定行原文
	cur := 1
	start := 0
	for i := 0; i < len(p.data); i++ {
		if cur == line {
			start = i
			break
		}
		if p.data[i] == '\n' {
			cur++
		}
	}
	if cur != line {
		return ""
	}
	end := start
	for end < len(p.data) && p.data[end] != '\n' && p.data[end] != '\r' {
		end++
	}
	return string(p.data[start:end])
}

// lineEnd 返回 from 之后行尾信息：tail 区间终点与 eol。
// 行内只允许空白与注释；出现其他实质 token（同行第二个 kv 等）报错。
func (p *parser) lineEnd(from int) (int, string, int, *ParseError) {
	j := from
	for j < len(p.toks) {
		k := p.toks[j].kind
		if k == tWhitespace || k == tComment {
			j++
			continue
		}
		break
	}
	if j < len(p.toks) && p.toks[j].kind == tNewline {
		tk := p.toks[j]
		return tk.off, tk.raw, j + 1, nil
	}
	if j >= len(p.toks) || p.toks[j].kind == tEOF {
		// 文件末尾无换行：tail 到最后一个跳过 token 的结束
		if j > from {
			last := p.toks[j-1]
			return last.off + len(last.raw), "", j, nil
		}
		tk := p.toks[from-1]
		return tk.off + len(tk.raw), "", from, nil
	}
	return 0, "", 0, p.failAt(&p.toks[j], "unexpected token on same line")
}

func (p *parser) run() (*Document, *ParseError) {
	p.cur = p.doc.root
	i := 0
	if len(p.toks) > 0 && p.toks[0].kind == tBOM {
		p.doc.bom = []byte(p.toks[0].raw)
		p.prevEnd = p.toks[0].off + len(p.toks[0].raw)
		i = 1
	}
	for i < len(p.toks) {
		tk := &p.toks[i]
		switch tk.kind {
		case tEOF:
			p.doc.trailing = string(p.data[p.prevEnd:])
			p.doc.root.d.tail = p.doc.trailing // dominantEOL 统计可见（root 无表头不渲染）
			return p.doc, nil
		case tNewline, tWhitespace, tComment:
			i++
		case tTableHeader:
			ni, perr := p.handleHeader(i)
			if perr != nil {
				return nil, perr
			}
			i = ni
		case tKey:
			ni, perr := p.handleKV(i)
			if perr != nil {
				return nil, perr
			}
			i = ni
		default:
			return nil, p.failAt(tk, "unexpected token")
		}
	}
	p.doc.trailing = string(p.data[p.prevEnd:])
	return p.doc, nil
}

// handleHeader 处理表头行 [path] / [[path]]，返回下一 token 索引。
func (p *parser) handleHeader(i int) (int, *ParseError) {
	tk := &p.toks[i]
	tailEnd, eol, next, perr := p.lineEnd(i + 1)
	if perr != nil {
		return 0, perr
	}
	d := decor{
		prefix: string(p.data[p.prevEnd:tk.off]),
		keyRaw: headerPathRaw(tk),
		tail:   string(p.data[tk.off+len(tk.raw) : tailEnd]),
		eol:    eol,
		seq:    float64(tk.line),
	}
	p.prevEnd = tailEnd + len(eol)

	t := p.doc.root
	segBase := ""
	for idx, part := range tk.parts {
		last := idx == len(tk.parts)-1
		existing, ok := t.items[part.name]
		if !ok {
			if last && tk.aot {
				arr := &Array{aot: true, owner: t}
				nt := &Table{items: map[string]Item{}, hasHeader: true, headerIsAoT: true, d: d}
				nt.pathSeg = joinPathSeg(segBase, tk.parts[:idx+1])
				arr.items = append(arr.items, nt)
				t.set(part.name, arr)
				p.cur = nt
				return next, nil
			}
			nt := &Table{items: map[string]Item{}, implicit: !last, owner: t}
			nt.pathSeg = joinPathSeg(segBase, tk.parts[:idx+1])
			if last {
				nt.hasHeader = true
				nt.d = d
			}
			t.set(part.name, nt)
			t = nt
			continue
		}
		switch e := existing.(type) {
		case *Table:
			if last {
				if tk.aot {
					return 0, p.failAt(tk, "cannot redefine table as array of tables")
				}
				if e.hasHeader {
					return 0, p.failAt(tk, "table redefined")
				}
				if e.dottedCreated {
					return 0, p.failAt(tk, "table defined by dotted key cannot be redefined by header")
				}
				e.implicit = false
				e.hasHeader = true
				e.d = d
			}
			// 中间段穿过 dotted 创建的表合法（可加子表，spec table-9）
			t = e
		case *Array:
			if !e.aot {
				return 0, p.failAt(tk, "key conflicts with existing value")
			}
			if last {
				if !tk.aot {
					return 0, p.failAt(tk, "cannot redefine array of tables as table")
				}
				e.owner = t
				nt := &Table{items: map[string]Item{}, hasHeader: true, headerIsAoT: true, d: d}
				e.items = append(e.items, nt)
				p.cur = nt
				return next, nil
			}
			// 深路径穿过 AoT：进入最后一个元素
			if len(e.items) == 0 {
				return 0, p.failAt(tk, "invalid array of tables")
			}
			t = e.items[len(e.items)-1].(*Table)
		default:
			return 0, p.failAt(tk, "key conflicts with existing value")
		}
	}
	p.cur = t
	return next, nil
}

// dottedKeyBase 返回当前表作为 dotted 前缀的基路径。
func (t *Table) dottedKeyBase() string {
	if t.hasHeader && t.d.keyRaw != "" {
		return t.d.keyRaw
	}
	return t.pathSeg
}

func joinPathSeg(base string, parts []keyPart) string {
	out := base
	for _, p := range parts {
		if out != "" {
			out += "."
		}
		out += p.raw
	}
	return out
}

func headerPathRaw(tk *token) string {
	if tk.aot {
		return tk.raw[2 : len(tk.raw)-2]
	}
	return tk.raw[1 : len(tk.raw)-1]
}

// handleKV 处理键值行（含 dotted key 展开），返回下一 token 索引。
func (p *parser) handleKV(i int) (int, *ParseError) {
	key := &p.toks[i]
	// 找 tEquals（间隔只允许 tWhitespace）
	j := i + 1
	for j < len(p.toks) && p.toks[j].kind == tWhitespace {
		j++
	}
	if j >= len(p.toks) || p.toks[j].kind != tEquals {
		return 0, p.failAt(key, "expected '=' after key")
	}
	j++
	for j < len(p.toks) && p.toks[j].kind == tWhitespace {
		j++
	}
	if j >= len(p.toks) || p.toks[j].kind != tValue {
		eq := &p.toks[j-1]
		pe := p.failAt(eq, "expected value after '='")
		pe.Col = eq.col + 1
		return 0, pe
	}
	val := &p.toks[j]
	tailEnd, eol, next, perr := p.lineEnd(j + 1)
	if perr != nil {
		return 0, perr
	}

	valNode, perr2 := buildScalar(*val)
	if perr2 != nil {
		return 0, perr2
	}
	dec := decor{
		prefix: string(p.data[p.prevEnd:key.off]),
		keyRaw: key.raw,
		sep:    string(p.data[key.off+len(key.raw) : val.off]),
		tail:   string(p.data[val.off+len(val.raw) : tailEnd]),
		eol:    eol,
		dotted: len(key.parts) > 1,
		seq:    float64(key.line),
	}
	setNodeDecor(valNode, dec)
	p.prevEnd = tailEnd + len(eol)

	// dotted 展开。中间段只能穿过 dotted 创建的表：任何表头（显式或
	// 隐式）定义过的表都不能被 dotted key 扩展（toml-test issue 846）。
	t := p.cur
	segBase := t.dottedKeyBase()
	for idx := 0; idx < len(key.parts)-1; idx++ {
		name := key.parts[idx].name
		existing, ok := t.items[name]
		if !ok {
			nt := &Table{items: map[string]Item{}, implicit: true, dottedCreated: true, owner: t}
			nt.pathSeg = joinPathSeg(segBase, key.parts[:idx+1])
			t.set(name, nt)
			t = nt
			continue
		}
		st, isTable := existing.(*Table)
		if !isTable || st.hasHeader || (st.implicit && !st.dottedCreated) {
			return 0, p.failAt(key, "dotted key cannot extend existing table")
		}
		t = st
	}
	leaf := key.parts[len(key.parts)-1].name
	if _, exists := t.items[leaf]; exists {
		return 0, p.failAt(key, "duplicate key")
	}
	t.set(leaf, valNode)
	return next, nil
}

func setNodeDecor(it Item, d decor) {
	switch e := it.(type) {
	case *Table:
		e.d = d
	case *InlineTable:
		e.d = d
	case *Array:
		e.d = d
	case *String:
		e.d = d
	case *Integer:
		e.d = d
	case *Float:
		e.d = d
	case *Boolean:
		e.d = d
	case *Datetime:
		e.d = d
	}
}

// buildScalar 从值 token 构造节点，委托 buildValue。
func buildScalar(tk token) (Item, *ParseError) {
	return buildValue(tk.raw, tk.vkind, tk.line, tk.col)
}

func parseInt64(s string) (int64, bool) {
	body := s
	base := 10
	switch {
	case reHexInt.MatchString(s):
		body, base = s[2:], 16
	case reOctInt.MatchString(s):
		body, base = s[2:], 8
	case reBinInt.MatchString(s):
		body, base = s[2:], 2
	case reDecInt.MatchString(s):
	default:
		return 0, false
	}
	body = strings.ReplaceAll(body, "_", "")
	v, err := strconv.ParseInt(body, base, 64)
	return v, err == nil
}

func parseFloat64(s string) (float64, bool) {
	switch s {
	case "inf", "+inf":
		return math.Inf(1), true
	case "-inf":
		return math.Inf(-1), true
	case "nan", "+nan", "-nan":
		return math.NaN(), true
	}
	if !reFloat.MatchString(s) || reDecInt.MatchString(s) {
		return 0, false
	}
	t := strings.ReplaceAll(s, "_", "")
	v, err := strconv.ParseFloat(t, 64)
	return v, err == nil
}

// TOML 1.0 数字形态：下划线必须夹在数字之间，进制前缀无符号，
// 十进制禁止前导零。
var (
	reDecInt = regexp.MustCompile(`^[+-]?(0|[1-9](_?[0-9])*)$`)
	reHexInt = regexp.MustCompile(`^0x[0-9A-Fa-f](_?[0-9A-Fa-f])*$`)
	reOctInt = regexp.MustCompile(`^0o[0-7](_?[0-7])*$`)
	reBinInt = regexp.MustCompile(`^0b[01](_?[01])*$`)
	reFloat  = regexp.MustCompile(`^[+-]?(0|[1-9](_?[0-9])*)(\.[0-9](_?[0-9])*)?([eE][+-]?[0-9](_?[0-9])*)?$`)
)

// decodeString 解码字符串三种形态的值。raw 不是引号形态（非法裸词）
// 返回 false。
func decodeString(raw string) (string, bool) {
	switch {
	case strings.HasPrefix(raw, `"""`) && strings.HasSuffix(raw, `"""`) && len(raw) >= 6:
		if trailingQuoteRun(raw[3:len(raw)-3], '"') > 2 {
			return "", false
		}
		body := raw[3 : len(raw)-3]
		body = trimFirstNewline(body)
		body = foldLineContinuations(body)
		return decodeEscapes(body)
	case strings.HasPrefix(raw, "'''") && strings.HasSuffix(raw, "'''") && len(raw) >= 6:
		if trailingQuoteRun(raw[3:len(raw)-3], '\'') > 2 {
			return "", false
		}
		body := raw[3 : len(raw)-3]
		return trimFirstNewline(body), true
	case strings.HasPrefix(raw, `"`):
		if len(raw) < 2 || !strings.HasSuffix(raw, `"`) {
			return "", false
		}
		return decodeEscapes(raw[1 : len(raw)-1])
	case len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'':
		return raw[1 : len(raw)-1], true
	default:
		return "", false
	}
}

func trailingQuoteRun(s string, q byte) int {
	n := 0
	i := len(s) - 1
	for i >= 0 && s[i] == q {
		n++
		i--
	}
	// 基本串里尾 run 最前的引号可能是被反斜杠转义的，不计入
	if q == '"' && n > 0 && i >= 0 && s[i] == '\\' {
		bs := 0
		for i-bs >= 0 && s[i-bs] == '\\' {
			bs++
		}
		if bs%2 == 1 {
			n--
		}
	}
	return n
}

func trimFirstNewline(s string) string {
	if strings.HasPrefix(s, "\r\n") {
		return s[2:]
	}
	if strings.HasPrefix(s, "\n") {
		return s[1:]
	}
	return s
}

// foldLineContinuations 折叠多行基本串的续行。转义感知：
// 反斜杠后跟非空白即转义对（两字符整体保留，后续反斜杠不受影响）；
// 反斜杠后全是空白且含换行则是续行，吃到下一个非空白字符。
func foldLineContinuations(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '\\' {
			sb.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			sb.WriteByte(c)
			i++
			continue
		}
		next := s[i+1]
		switch {
		case next == ' ' || next == '\t' || next == '\r':
			j := i + 1
			sawNewline := false
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r' || s[j] == '\n') {
				if s[j] == '\n' {
					sawNewline = true
				}
				j++
			}
			if sawNewline {
				i = j // 续行：整段空白删除
				continue
			}
			sb.WriteByte(c) // 非续行，保留给转义校验报错
			i++
		case next == '\n':
			j := i + 2
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r' || s[j] == '\n') {
				j++
			}
			i = j
			continue
		default:
			sb.WriteByte(c)
			sb.WriteByte(next) // 转义对整体保留
			i += 2
		}
	}
	return sb.String()
}

func decodeEscapes(s string) (string, bool) {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			sb.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", false
		}
		switch s[i] {
		case 'b':
			sb.WriteByte('\b')
		case 't':
			sb.WriteByte('\t')
		case 'n':
			sb.WriteByte('\n')
		case 'f':
			sb.WriteByte('\f')
		case 'r':
			sb.WriteByte('\r')
		case '"':
			sb.WriteByte('"')
		case '\\':
			sb.WriteByte('\\')
		case 'u', 'U':
			n := 4
			if s[i] == 'U' {
				n = 8
			}
			if i+n+1 > len(s) {
				return "", false
			}
			hex := s[i+1 : i+1+n]
			cp, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return "", false
			}
			if cp >= 0xD800 && cp <= 0xDFFF || cp > 0x10FFFF {
				return "", false
			}
			sb.WriteRune(rune(cp))
			i += n
		default:
			return "", false
		}
	}
	return sb.String(), true
}
