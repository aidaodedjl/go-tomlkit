package tomlkit

import (
	"fmt"
	"strings"
)

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

// Detailed 返回多行人类可读错误：出错行原文与列指示（^），适合直接
// 展示给最终用户。
func (e *ParseError) Detailed() string {
	if e.Snippet == "" {
		return e.Error()
	}
	pad := strings.Repeat(" ", max(0, e.Col-1))
	return fmt.Sprintf("tomlkit: %s\n    %s\n    %s^", e.Msg, e.Snippet, pad)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
