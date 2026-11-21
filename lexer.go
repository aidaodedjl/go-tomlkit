package tomlkit

// lexer 把 TOML 文本切分为 token。token 保留原文区间与行列位置，
// parser 据此重组 decor 并构造文档树。
//
// 值 token 的扫描在本文件是简化版：扫到行尾再回溯剥离空白与注释，
// 字符串与复合值的完整匹配（转义、跨行数组、内联表）由 Task 3 完善。

import (
	"strings"
	"unicode/utf8"
)

type tokenKind int8

const (
	tEOF tokenKind = iota
	tBOM
	tNewline // \n 或 \r\n
	tComment // # 到行尾（不含换行）
	tWhitespace
	tKey         // 裸键或引号键
	tEquals      // =
	tTableHeader // [path] 或 [[path]]
	tValue       // 值域原文（简化版：到行尾）
)

type token struct {
	kind tokenKind
	raw  string // 原文
	line int    // 1-based
	col  int    // 1-based
	off  int    // 0-based 字节偏移
	// tKey 时为解码后键名；tTableHeader 时为最后一段
	str string
	// tTableHeader 时为路径段
	parts []keyPart
	aot   bool      // [[x]] 双方括号形态
	vkind valueKind // tValue 时为值形态初判
}

// valueKind 是值 token 的形态分类。精确解析（进制、转义合法性）由 parser 验证。
type valueKind int8

const (
	vString valueKind = iota
	vInteger
	vFloat
	vBoolean
	vDatetime
	vInlineTable
	vArray
)

type keyPart struct {
	name string // 解码后段名
	raw  string // 段原文书写
}

func isBareKeyByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

func lex(data []byte) ([]token, *ParseError) {
	lx := &lexer{data: data, line: 1, col: 1, atLineStart: true}
	if perr := lx.checkUTF8(); perr != nil {
		return nil, perr
	}
	lx.run()
	if lx.perr != nil {
		return nil, lx.perr
	}
	return lx.toks, nil
}

type lexer struct {
	data         []byte
	pos          int
	line, col    int
	tokLine      int // 当前 token 起点行列
	tokCol       int
	toks         []token
	perr         *ParseError
	atLineStart  bool // 处于行首（只允许空白/注释/表头/键）
	expectingVal bool // 等号之后，裸词与引号串是值不是键
}

func (lx *lexer) emit(kind tokenKind, start int) {
	lx.toks = append(lx.toks, token{
		kind: kind, raw: string(lx.data[start:lx.pos]),
		line: lx.tokLine, col: lx.tokCol, off: start,
	})
}
func (lx *lexer) cur() byte { return lx.data[lx.pos] }
func (lx *lexer) eof() bool { return lx.pos >= len(lx.data) }
func (lx *lexer) advance() {
	if lx.cur() == '\n' {
		lx.line++
		lx.col = 1
	} else {
		lx.col++
	}
	lx.pos++
}

func (lx *lexer) fail(msg string) {
	// Snippet：当前行原文（不含换行）
	ls := lx.pos
	for ls > 0 && lx.data[ls-1] != '\n' {
		ls--
	}
	le := lx.pos
	for le < len(lx.data) && lx.data[le] != '\n' && lx.data[le] != '\r' {
		le++
	}
	lx.perr = &ParseError{Line: lx.line, Col: lx.col, Offset: lx.pos, Snippet: string(lx.data[ls:le]), Msg: msg}
}

func (lx *lexer) markPos() { lx.tokLine, lx.tokCol = lx.line, lx.col }

func (lx *lexer) run() {
	for lx.perr == nil && !lx.eof() {
		lx.markPos()
		start := lx.pos
		b := lx.cur()
		switch {
		case lx.pos == 0 && lx.hasBOMAt(lx.pos):
			lx.pos += 3
			lx.col += 3
			lx.emit(tBOM, start)
		case lx.hasBOMAt(lx.pos):
			lx.fail("BOM is only allowed at the start of the document")
			return
		case b == '\n':
			lx.advance()
			lx.emit(tNewline, start)
			lx.atLineStart = true
		case b == '\r':
			if lx.pos+1 < len(lx.data) && lx.data[lx.pos+1] == '\n' {
				lx.advance()
				lx.advance()
				lx.emit(tNewline, start)
				lx.atLineStart = true
			} else {
				lx.fail("bare carriage return is not allowed")
				return
			}
		case b == ' ' || b == '\t':
			for !lx.eof() && (lx.cur() == ' ' || lx.cur() == '\t') {
				lx.advance()
			}
			lx.emit(tWhitespace, start)
		case b == '#':
			for !lx.eof() && lx.cur() != '\n' && lx.cur() != '\r' {
				if lx.cur() != '\t' && isControlByte(lx.cur()) {
					lx.fail("control character in comment")
					return
				}
				lx.advance()
			}
			lx.emit(tComment, start)
		case b == '=':
			lx.advance()
			lx.emit(tEquals, start)
			lx.atLineStart = false
			lx.expectingVal = true
		case b == '[' && lx.atLineStart:
			lx.lexHeader(start)
			if lx.perr != nil {
				return
			}
			lx.atLineStart = false
		case isBareKeyByte(b) && !lx.expectingVal:
			lx.lexKey(start, false)
			if lx.perr != nil {
				return
			}
		case (b == '"' || b == '\'') && !lx.expectingVal:
			lx.lexKey(start, true)
			if lx.perr != nil {
				return
			}
		default:
			lx.lexValue(start)
			if lx.perr != nil {
				return
			}
			lx.atLineStart = false
			lx.expectingVal = false
		}
	}
	if lx.perr == nil {
		lx.markPos()
		lx.emit(tEOF, lx.pos)
	}
}

// lexKey 扫描键（含 dotted 形态 a.b."c"）。raw 覆盖含点与点周空白的原文，
// parts 携带解码后的各段。
func (lx *lexer) lexKey(start int, quoted bool) {
	var parts []keyPart
	for {
		if quoted {
			raw, dec, ok := lx.lexQuotedString()
			if !ok {
				return
			}
			parts = append(parts, keyPart{name: dec, raw: raw})
		} else {
			s := lx.pos
			for !lx.eof() && isBareKeyByte(lx.cur()) {
				lx.advance()
			}
			raw := string(lx.data[s:lx.pos])
			parts = append(parts, keyPart{name: raw, raw: raw})
		}
		// 前瞻（不推进）：空白* '.' 才是 dotted 续段
		j := lx.pos
		for j < len(lx.data) && (lx.data[j] == ' ' || lx.data[j] == '\t') {
			j++
		}
		if j < len(lx.data) && lx.data[j] == '.' && !lx.expectingVal {
			for lx.pos <= j {
				lx.advance()
			}
			for !lx.eof() && (lx.cur() == ' ' || lx.cur() == '\t') {
				lx.advance()
			}
			if lx.eof() || !(lx.cur() == '"' || lx.cur() == '\'' || isBareKeyByte(lx.cur())) {
				lx.fail("invalid dotted key")
				return
			}
			quoted = lx.cur() == '"' || lx.cur() == '\''
			continue
		}
		break
	}
	lx.toks = append(lx.toks, token{
		kind: tKey, raw: string(lx.data[start:lx.pos]), line: lx.tokLine, col: lx.tokCol,
		off: start, parts: parts, str: parts[len(parts)-1].name,
	})
	lx.atLineStart = false
}

// checkUTF8 校验整个输入是合法 UTF-8（TOML 要求）。
func (lx *lexer) checkUTF8() *ParseError {
	line := 1
	for i := 0; i < len(lx.data); {
		r, size := utf8.DecodeRune(lx.data[i:])
		if r == utf8.RuneError && size <= 1 {
			lx.pos = i
			lx.line = line
			lx.fail("invalid UTF-8")
			return lx.perr
		}
		if r == '\n' {
			line++
		}
		i += size
	}
	return nil
}

// isControlByte 报告 b 是否为 TOML 禁止的原始控制字符（tab 允许）。
func isControlByte(b byte) bool {
	return b <= 0x08 || b == 0x0B || b == 0x0C || (b >= 0x0E && b <= 0x1F) || b == 0x7F
}

func (lx *lexer) hasBOMAt(pos int) bool {
	return pos+3 <= len(lx.data) && lx.data[pos] == 0xEF && lx.data[pos+1] == 0xBB && lx.data[pos+2] == 0xBF
}

// lexQuotedString 扫描一个引号字符串（键用），返回原文与解码值。
// 只处理单行形态；多行字符串作为值时不会被走到这里（值扫描到行尾）。
func (lx *lexer) lexQuotedString() (raw, decoded string, ok bool) {
	start := lx.pos
	quote := lx.cur()
	lx.advance()
	for !lx.eof() {
		b := lx.cur()
		if b == '\n' || b == '\r' {
			lx.fail("unterminated quoted string")
			return "", "", false
		}
		if b != '\t' && isControlByte(b) {
			lx.fail("control character in quoted key")
			return "", "", false
		}
		if quote == '"' && b == '\\' && lx.pos+1 < len(lx.data) {
			lx.advance()
			lx.advance()
			continue
		}
		if b == quote {
			lx.advance()
			raw = string(lx.data[start:lx.pos])
			var ok bool
			decoded, ok = decodeQuotedKey(raw, quote)
			if !ok {
				lx.fail("invalid escape in quoted key")
				return "", "", false
			}
			return raw, decoded, true
		}
		lx.advance()
	}
	lx.fail("unterminated quoted string")
	return "", "", false
}

// decodeQuotedKey 解码键字符串。字面串原样去引号；基本串走全集转义，
// 非法转义返回 false。
func decodeQuotedKey(raw string, quote byte) (string, bool) {
	inner := raw[1 : len(raw)-1]
	if quote == '\'' {
		return inner, true
	}
	return decodeEscapes(inner)
}

// lexHeader 扫描 [path] 或 [[path]] 表头，解析路径段。
func (lx *lexer) lexHeader(start int) {
	lx.advance() // 消耗 '['
	aot := false
	if !lx.eof() && lx.cur() == '[' {
		aot = true
		lx.advance()
	}
	// 扫到匹配闭合
	closer := "]"
	if aot {
		closer = "]]"
	}
	for {
		if lx.eof() {
			lx.fail("unterminated table header")
			return
		}
		if lx.cur() == '\n' || lx.cur() == '\r' {
			lx.fail("table header must be on a single line")
			return
		}
		if lx.cur() == '"' || lx.cur() == '\'' {
			// 引号段内的 ] 不当 closer
			end, ok := findStringEnd(lx.data, lx.pos)
			if !ok {
				lx.fail("invalid quoted key in table header")
				return
			}
			for lx.pos < end {
				lx.advance()
			}
			continue
		}
		if lx.hasCloserAt(lx.pos, closer) {
			break
		}
		lx.advance()
	}
	headerRaw := string(lx.data[start : lx.pos+len(closer)])
	for i := 0; i < len(closer); i++ {
		lx.advance()
	}
	parts, ok := parseHeaderPath(headerRaw)
	if !ok {
		lx.fail("invalid table header path")
		return
	}
	lx.toks = append(lx.toks, token{kind: tTableHeader, raw: headerRaw, line: lx.tokLine, col: lx.tokCol, off: start, parts: parts, aot: aot, str: parts[len(parts)-1].name})
}

func (lx *lexer) hasCloserAt(pos int, closer string) bool {
	return pos+len(closer) <= len(lx.data) && string(lx.data[pos:pos+len(closer)]) == closer
}

// parseHeaderPath 解析表头原文 "[a.b]" / "[[a]]" 为路径段。
func parseHeaderPath(raw string) ([]keyPart, bool) {
	if strings.HasPrefix(raw, "[[") {
		raw = raw[2 : len(raw)-2]
	} else {
		raw = raw[1 : len(raw)-1]
	}
	seg := &segmenter{src: raw}
	var parts []keyPart
	for {
		seg.skipBlank()
		if seg.eof() {
			break
		}
		p, ok2 := seg.next()
		if !ok2 {
			return nil, false
		}
		parts = append(parts, p)
		seg.skipBlank()
		if seg.eof() {
			break
		}
		if seg.src[seg.pos] != '.' {
			return nil, false
		}
		seg.pos++
		seg.skipBlank()
		if seg.eof() {
			return nil, false // 尾点：点后必须还有段
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}

type segmenter struct {
	src string
	pos int
}

func (s *segmenter) eof() bool { return s.pos >= len(s.src) }
func (s *segmenter) skipBlank() {
	for !s.eof() && (s.src[s.pos] == ' ' || s.src[s.pos] == '\t') {
		s.pos++
	}
}

func (s *segmenter) next() (keyPart, bool) {
	start := s.pos
	if s.src[s.pos] == '"' || s.src[s.pos] == '\'' {
		quote := s.src[s.pos]
		s.pos++
		for !s.eof() && s.src[s.pos] != quote {
			if quote == '"' && s.src[s.pos] == '\\' {
				s.pos++
			}
			s.pos++
		}
		if s.eof() {
			return keyPart{}, false
		}
		s.pos++
		raw := s.src[start:s.pos]
		name, ok2 := decodeQuotedKey(raw, quote)
		if !ok2 {
			return keyPart{}, false
		}
		return keyPart{name: name, raw: raw}, true
	}
	for !s.eof() && isBareKeyByte(s.src[s.pos]) {
		s.pos++
	}
	if s.pos == start {
		return keyPart{}, false
	}
	raw := s.src[start:s.pos]
	return keyPart{name: raw, raw: raw}, true
}

// lexValue 扫描完整值域：字符串（三种形态）、内联表、数组（可跨行）、
// 裸值（数字/bool/datetime，到空白、注释或行尾）。
func (lx *lexer) lexValue(start int) {
	var end int
	b := lx.data[start]
	switch {
	case b == '"' || b == '\'':
		var ok bool
		end, ok = findStringEnd(lx.data, start)
		if !ok {
			lx.fail("unterminated string")
			return
		}
	case b == '[':
		end = lx.scanBracketed(start, '[', ']')
		if lx.perr != nil {
			return
		}
	case b == '{':
		end = lx.scanBracketed(start, '{', '}')
		if lx.perr != nil {
			return
		}
	default:
		end = lx.scanBare(start)
		if lx.perr != nil {
			return
		}
	}
	lx.toks = append(lx.toks, token{
		kind: tValue, raw: string(lx.data[start:end]), vkind: classifyValue(lx.data[start:end]),
		line: lx.tokLine, col: lx.tokCol, off: start,
	})
	for lx.pos < end {
		lx.advance()
	}
}

// findStringEnd 返回字符串（含闭合引号）的结束位置。
// 处理三种形态：基本串、字面串、多行（""" / ”'）。
func findStringEnd(data []byte, start int) (int, bool) {
	q := data[start]
	// 三引号检测
	if start+3 <= len(data) && data[start+1] == q && data[start+2] == q {
		p := start + 3
		for p < len(data) {
			if q == '"' && data[p] == '\\' {
				p += 2
				continue
			}
			if data[p] != '\n' && data[p] != '\r' && data[p] != '\t' && isControlByte(data[p]) {
				return 0, false
			}
			if data[p] == '\r' && (p+1 >= len(data) || data[p+1] != '\n') {
				return 0, false
			}
			if data[p] == q {
				// 贪心吃连续引号 run，末 3 个为闭合（TOML 允许内容含至多 2 个紧贴引号；
				// 更多由 parser 报错，lexer 只定界）
				runEnd := p
				for runEnd < len(data) && data[runEnd] == q {
					runEnd++
				}
				if runEnd-p >= 3 {
					return runEnd, true
				}
				p = runEnd
				continue
			}
			p++
		}
		return 0, false
	}
	// 单行形态
	p := start + 1
	for p < len(data) {
		b := data[p]
		if q == '"' && b == '\\' {
			p += 2
			continue
		}
		if b != '\t' && isControlByte(b) {
			return 0, false
		}
		if b == q {
			return p + 1, true
		}
		if b == '\n' || b == '\r' {
			return 0, false
		}
		p++
	}
	return 0, false
}

// scanBracketed 扫描括号匹配的复合值（数组或内联表），可跨行，
// 内部允许注释与字符串（字符串与注释里的括号不计入深度）。
func (lx *lexer) scanBracketed(start int, open, close byte) int {
	data := lx.data
	depth := 1
	p := start + 1
	for p < len(data) {
		b := data[p]
		switch {
		case b == '"' || b == '\'':
			end, ok := findStringEnd(data, p)
			if !ok {
				lx.fail("unterminated string in compound value")
				return 0
			}
			p = end
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
	lx.fail("unterminated compound value")
	return 0
}

// scanBare 扫描裸值（数字、bool、datetime、inf/nan），终止于
// 空白、注释、换行或复合值分隔符。
func (lx *lexer) scanBare(start int) int {
	p := start
	for p < len(lx.data) {
		b := lx.data[p]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '#' || b == ',' || b == '}' || b == ']' {
			break
		}
		p++
	}
	if p == start {
		lx.fail("invalid value")
		return 0
	}
	// 空格分隔的 local datetime：1979-05-27 07:32:00（TOML 允许一个空格替代 T）
	if p+3 <= len(lx.data) && lx.data[p] == ' ' && isDigit(lx.data[p+1]) && isDigit(lx.data[p+2]) && p+3 < len(lx.data) && lx.data[p+3] == ':' {
		if q := lx.scanBare(p + 1); lx.perr == nil {
			return q
		}
		lx.perr = nil
	}
	return p
}

// classifyValue 对裸值与字符串做形态初判。
func classifyValue(raw []byte) valueKind {
	s := string(raw)
	switch {
	case s[0] == '{':
		return vInlineTable
	case s[0] == '[':
		return vArray
	case s == "true" || s == "false":
		return vBoolean
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0o"), strings.HasPrefix(s, "0b"),
		strings.HasPrefix(s, "-0x"), strings.HasPrefix(s, "-0o"), strings.HasPrefix(s, "-0b"),
		strings.HasPrefix(s, "+0x"), strings.HasPrefix(s, "+0o"), strings.HasPrefix(s, "+0b"):
		return vInteger
	case len(s) >= 5 && s[4] == '-' && isDigit(s[0]):
		return vDatetime // 1979-05-27…
	case len(s) >= 3 && s[2] == ':' && isDigit(s[0]) && isDigit(s[1]):
		return vDatetime // 07:32:00
	case s == "inf" || s == "+inf" || s == "-inf" || s == "nan" || s == "+nan" || s == "-nan":
		return vFloat
	case strings.ContainsAny(s, ".eE") && isDigitOrSign(s[0]):
		// datetime 形态已被上面分支截获，含 . e E 的数字即浮点
		return vFloat
	case isDigitOrSign(s[0]):
		return vInteger
	default:
		return vString
	}
}

func isDigit(b byte) bool       { return b >= '0' && b <= '9' }
func isDigitOrSign(b byte) bool { return isDigit(b) || b == '+' || b == '-' }
