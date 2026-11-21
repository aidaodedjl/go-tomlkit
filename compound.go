package tomlkit

// 复合值（数组、内联表）的成员解析：对 raw 做递归下降，
// 复用 lexer 的字符串定界与裸值分类。

import (
	"regexp"
	"strings"
	"time"
)

// buildValue 从值原文构造节点（标量与复合）。
func buildValue(raw string, kind valueKind, line, col int) (Item, *ParseError) {
	bad := func(msg string) *ParseError {
		return &ParseError{Line: line, Col: col, Msg: msg}
	}
	switch kind {
	case vString:
		v, ok := decodeString(raw)
		if !ok {
			return nil, bad("invalid string")
		}
		return &String{v: v, raw: raw}, nil
	case vInteger:
		v, ok := parseInt64(raw)
		if !ok {
			return nil, bad("invalid integer")
		}
		return &Integer{v: v, raw: raw}, nil
	case vFloat:
		v, ok := parseFloat64(raw)
		if !ok {
			return nil, bad("invalid float")
		}
		return &Float{v: v, raw: raw}, nil
	case vBoolean:
		return &Boolean{v: raw == "true", raw: raw}, nil
	case vDatetime:
		return buildDatetime(raw, line, col)
	case vInlineTable:
		return parseInlineTableRaw(raw, line, col)
	case vArray:
		return parseArrayRaw(raw, line, col)
	}
	return nil, bad("unreachable value kind")
}

var datetimeLayouts = []struct {
	layout string
	kind   DatetimeKind
}{
	{time.RFC3339Nano, KindOffsetDatetime},
	{"2006-01-02 15:04:05.999999999Z07:00", KindOffsetDatetime},
	{"2006-01-02T15:04:05.999999999", KindLocalDatetime},
	{"2006-01-02 15:04:05.999999999", KindLocalDatetime},
	{"2006-01-02", KindLocalDate},
	{"15:04:05.999999999", KindLocalTime},
}

func buildDatetime(raw string, line, col int) (*Datetime, *ParseError) {
	// ABNF 大小写不敏感：t/T、z/Z 都合法，Go 布局大小写敏感，先规范化
	norm := strings.Replace(raw, "t", "T", 1)
	norm = strings.Replace(norm, "z", "Z", 1)
	var kind DatetimeKind
	switch {
	case reOffsetDatetime.MatchString(norm):
		kind = KindOffsetDatetime
	case reLocalDatetime.MatchString(norm):
		kind = KindLocalDatetime
	case reLocalDate.MatchString(norm):
		kind = KindLocalDate
	case reLocalTime.MatchString(norm):
		kind = KindLocalTime
	default:
		return nil, &ParseError{Line: line, Col: col, Msg: "invalid datetime: " + raw}
	}
	for _, l := range datetimeLayouts {
		if l.kind != kind {
			continue
		}
		if ts, err := time.Parse(l.layout, norm); err == nil {
			return &Datetime{Kind: l.kind, Time: ts, raw: raw}, nil
		}
	}
	return nil, &ParseError{Line: line, Col: col, Msg: "invalid datetime: " + raw}
}

var (
	reOffsetDatetime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$`)
	reLocalDatetime  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?$`)
	reLocalDate      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reLocalTime      = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d+)?$`)
)

// cs 是复合值内部的扫描器。
type cs struct {
	src        string
	pos        int
	line       int
	col        int
	pendingErr *ParseError
}

func (s *cs) fail(msg string) *ParseError {
	return &ParseError{Line: s.line, Col: s.col, Msg: msg}
}

func (s *cs) eof() bool { return s.pos >= len(s.src) }

func (s *cs) peek() byte { return s.src[s.pos] }

// skipTrivia 跳过空白与注释（控制字符非法，返回原文供上层报错标记），
// 返回跳过的原文。遇非法控制字符记录 pendingErr。
func (s *cs) skipTrivia() string {
	start := s.pos
	for !s.eof() {
		b := s.peek()
		if b == ' ' || b == '\t' || b == '\n' {
			s.advance()
			continue
		}
		if b == '\r' {
			if s.pos+1 < len(s.src) && s.src[s.pos+1] == '\n' {
				s.advance()
				s.advance()
				continue
			}
			s.pendingErr = s.fail("bare carriage return in compound value")
			return s.src[start:s.pos]
		}
		if b == '#' {
			for !s.eof() && s.peek() != '\n' {
				if s.peek() != '\t' && isControlByte(s.peek()) {
					s.pendingErr = s.fail("control character in comment")
					return s.src[start:s.pos]
				}
				s.advance()
			}
			continue
		}
		break
	}
	return s.src[start:s.pos]
}

func (s *cs) advance() {
	if s.src[s.pos] == '\n' {
		s.line++
		s.col = 1
	} else {
		s.col++
	}
	s.pos++
}

// scanValueRaw 扫描一个值的完整原文（复合上下文），终止于 , } ] 空白(非日期跟随) # 换行。
func (s *cs) scanValueRaw() (string, valueKind, bool) {
	start := s.pos
	data := []byte(s.src)
	b := s.src[s.pos]
	var end int
	switch {
	case b == '"' || b == '\'':
		e, ok := findStringEnd(data, s.pos)
		if !ok {
			return "", 0, false
		}
		end = e
	case b == '[':
		end = s.scanBracketed(']', '[')
	case b == '{':
		end = s.scanBracketed('}', '{')
	default:
		end = s.scanBareInCompound()
		if end < 0 {
			return "", 0, false
		}
		// 空格分隔 datetime 前瞻
		if end+3 < len(s.src) && s.src[end] == ' ' && isDigit(s.src[end+1]) && isDigit(s.src[end+2]) && s.src[end+3] == ':' {
			save := s.pos
			s.pos = end
			s.skipWsNoNewline()
			if s.pos < len(s.src) {
				s.pos = save
				sub := s.src[end+1:]
				subEnd := scanBareFromString(sub)
				if subEnd > 0 {
					end = end + 1 + subEnd
				}
			}
			s.pos = save
		}
	}
	raw := s.src[start:end]
	for s.pos < end {
		s.advance()
	}
	return raw, classifyValue([]byte(raw)), true
}

func (s *cs) skipWsNoNewline() string {
	start := s.pos
	for !s.eof() && (s.peek() == ' ' || s.peek() == '\t') {
		s.advance()
	}
	return s.src[start:s.pos]
}

// scanBracketed 复合上下文的括号匹配扫描（含字符串与注释感知）。
func (s *cs) scanBracketed(close, open byte) int {
	data := []byte(s.src)
	depth := 1
	p := s.pos + 1
	for p < len(data) {
		b := data[p]
		switch {
		case b == '"' || b == '\'':
			e, ok := findStringEnd(data, p)
			if !ok {
				return -1
			}
			p = e
		case b == '#':
			for p < len(data) && data[p] != '\n' {
				p++
			}
		case b == open:
			depth++
			p++
		case b == close:
			depth--
			if depth == 0 {
				return p + 1
			}
			p++
		default:
			p++
		}
	}
	return -1
}

// scanBareInCompound 扫描裸值终止符：, } ] 空白 # 换行。返回 -1 表示空值。
func (s *cs) scanBareInCompound() int {
	p := s.pos
	for p < len(s.src) {
		b := s.src[p]
		if b == ',' || b == '}' || b == ']' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '#' {
			break
		}
		p++
	}
	if p == s.pos {
		return -1
	}
	return p
}

func scanBareFromString(sub string) int {
	p := 0
	for p < len(sub) {
		b := sub[p]
		if b == ',' || b == '}' || b == ']' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '#' {
			break
		}
		p++
	}
	return p
}

func parseArrayRaw(raw string, line, col int) (*Array, *ParseError) {
	arr := &Array{}
	s := &cs{src: raw, line: line, col: col}
	s.pos = 1    // 跳过 '['
	tailEnd := 1 // 末元素 tail 结束位置（含尾逗号时为逗号前）
	for {
		elems := len(arr.items)
		var prefix string
		if elems == 0 {
			prefix = s.skipWsNoNewline()
			if !s.eof() && s.peek() == '#' {
				prefix += s.skipTrivia()
			} else if !s.eof() && (s.peek() == '\n' || s.peek() == '\r') {
				prefix += s.skipTrivia()
			}
		} else {
			prefix = s.skipTrivia()
		}
		if s.eof() {
			return nil, s.fail("unterminated array")
		}
		if s.peek() == ']' {
			// 顶部闭合：尾逗号路径（逗号已消耗，prefix 吃过的 trivia 归 closeRaw）
			if len(arr.items) == 0 {
				arr.closeRaw = s.src[1:s.pos]
			} else {
				arr.closeRaw = s.src[tailEnd:s.pos]
			}
			s.advance()
			break
		}
		vraw, kind, ok := s.scanValueRaw()
		if !ok || vraw == "" {
			return nil, s.fail("invalid array element")
		}
		item, perr := buildValue(vraw, kind, s.line, s.col)
		if perr != nil {
			return nil, perr
		}
		// tail：元素后到逗号/闭合括号
		tail := s.skipTrivia()
		tailEnd = s.pos
		arr.items = append(arr.items, item)
		arr.elems = append(arr.elems, arrayElem{prefix: prefix, tail: tail})
		if s.eof() {
			return nil, s.fail("unterminated array")
		}
		if s.peek() == ',' {
			s.advance()
			continue
		}
		if s.peek() == ']' {
			if len(arr.items) == 0 {
				arr.closeRaw = s.src[1:s.pos]
			} else {
				arr.closeRaw = s.src[tailEnd:s.pos]
			}
			s.advance()
			break
		}
		return nil, s.fail("expected ',' or ']' in array")
	}
	return arr, nil
}

func parseInlineTableRaw(raw string, line, col int) (*InlineTable, *ParseError) {
	it := &InlineTable{items: map[string]Item{}}
	s := &cs{src: raw, line: line, col: col}
	s.pos = 1 // 跳过 '{'
	it.pre = s.skipWsNoNewline()
	afterComma := false
	parts0 := []string{}
	var prevParent *Table // 上一成员的父表（组内分隔归属）
	pendingSepStart := -1 // 上一物理成员结束后挂起的分隔起点
	prevFirstSeg := ""
	for {
		if s.pendingErr != nil {
			return nil, s.pendingErr
		}
		_ = parts0
		if s.eof() {
			return nil, s.fail("unterminated inline table")
		}
		if s.peek() == '}' {
			if afterComma {
				return nil, s.fail("trailing comma in inline table")
			}
			s.advance()
			break
		}
		// 键（bare/quoted，可 dotted）
		keyStart := s.pos
		parts, ok := s.scanKey()
		parts0 = parts
		if !ok {
			return nil, s.fail("invalid key in inline table")
		}
		// 分隔提交：本成员是新逻辑键（首段不同）才提交上一键的分隔，
		// 区间到键名开始为止（不含键名）；dotted 组内不提交
		if pendingSepStart >= 0 {
			sep := s.src[pendingSepStart:keyStart]
			if parts[0] != prevFirstSeg {
				it.seps = append(it.seps, sep)
			} else if prevParent != nil {
				// dotted 组内：分隔挂成员父表（渲染端组内保真）
				prevParent.inlineSeps = append(prevParent.inlineSeps, sep)
			}
			pendingSepStart = -1
		}
		if !ok {
			return nil, s.fail("invalid key in inline table")
		}
		keyEnd := s.pos
		keyRawStr := s.src[keyStart:keyEnd]
		s.skipWsNoNewline()
		if s.eof() || s.peek() != '=' {
			return nil, s.fail("expected '=' in inline table")
		}
		s.advance()
		eqEnd := s.pos
		s.skipWsNoNewline()
		sepStr := s.src[keyEnd:eqEnd] + s.src[eqEnd:s.pos]
		vraw, kind, ok := s.scanValueRaw()
		if !ok || vraw == "" {
			return nil, s.fail("invalid value in inline table")
		}
		item, perr := buildValue(vraw, kind, s.line, s.col)
		if perr != nil {
			return nil, perr
		}
		// dotted 展开
		var cur tableLike = it
		for i := 0; i < len(parts)-1; i++ {
			existing, exists := cur.get(parts[i])
			if !exists {
				nt := &Table{items: map[string]Item{}, implicit: true, dottedCreated: true}
				if tb2, ok2 := cur.(*Table); ok2 {
					nt.pathSeg = joinKeyPath(tb2, parts[i])
				}
				cur.set(parts[i], nt)
				cur = nt
				continue
			}
			st, isT := existing.(*Table)
			if !isT {
				return nil, s.fail("dotted key conflicts in inline table")
			}
			cur = st
		}
		leaf := parts[len(parts)-1]
		if _, exists := cur.get(leaf); exists {
			return nil, s.fail("duplicate key in inline table")
		}
		// 成员 decor：keyRaw 原文 + sep（= 与前后空白）
		cur.set(leaf, item)
		if pt, ok3 := cur.(*Table); ok3 {
			prevParent = pt
		} else {
			prevParent = nil
		}
		setNodeDecor(item, decor{keyRaw: keyRawStr, sep: sepStr, dotted: len(parts) > 1})
		sepStart := s.pos
		s.skipWsNoNewline()
		if s.eof() {
			return nil, s.fail("unterminated inline table")
		}
		if s.peek() == ',' {
			s.advance()
			s.skipWsNoNewline()
			prevFirstSeg = parts0[0]
			pendingSepStart = sepStart
			afterComma = true
			continue
		}
		if s.peek() == '}' {
			// 尾分隔（末物理成员到 } 的原文）无条件保留——identity 依赖
			it.seps = append(it.seps, s.src[sepStart:s.pos])
			s.advance()
			break
		}
		return nil, s.fail("expected ',' or '}' in inline table")
	}
	return it, nil
}

// scanKey 扫描复合上下文中的键（含 dotted）。
func (s *cs) scanKey() ([]string, bool) {
	var parts []string
	for {
		if s.eof() {
			return nil, false
		}
		b := s.peek()
		switch {
		case b == '"' || b == '\'':
			e, ok := findStringEnd([]byte(s.src), s.pos)
			if !ok {
				return nil, false
			}
			raw := s.src[s.pos:e]
			name, ok2 := decodeQuotedKey(raw, b)
			if !ok2 {
				return nil, false
			}
			parts = append(parts, name)
			for s.pos < e {
				s.advance()
			}
		case isBareKeyByte(b):
			start := s.pos
			for !s.eof() && isBareKeyByte(s.peek()) {
				s.advance()
			}
			parts = append(parts, s.src[start:s.pos])
		default:
			return nil, false
		}
		s.skipWsNoNewline()
		if !s.eof() && s.peek() == '.' {
			s.advance()
			s.skipWsNoNewline()
			continue
		}
		return parts, true
	}
}
