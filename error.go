package tomlkit

import "fmt"

// ParseError 是解析错误，带位置信息与出错行原文。
type ParseError struct {
	Line    int    // 1-based
	Col     int    // 1-based
	Offset  int    // 0-based 字节偏移
	Snippet string // 出错行原文
	Msg     string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("tomlkit: line %d, col %d: %s: %q", e.Line, e.Col, e.Msg, e.Snippet)
}
