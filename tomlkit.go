// Package tomlkit 提供 TOML 的保格式（round-trip fidelity）读写。
//
// 解析得到保留全部格式信息（注释、空行、缩进、键序、引号风格）的文档树，
// 编辑后写回：未修改的部分逐字节不变，修改的部分按默认风格渲染。
package tomlkit

// Document 是一个 TOML 文档的根。
type Document struct {
	bom      []byte // 开头 BOM 原文，无则 nil
	root     *Table
	trailing string // 最后条目之后的文件尾原文（空行、注释）
}

// Parse 解析 TOML 文档。输入中的格式信息全部保留在返回的树里。
func Parse(data []byte) (*Document, error) {
	doc, perr := parse(data)
	if perr != nil {
		if perr.Snippet == "" && perr.Line > 0 {
			p := &parser{data: data}
			perr.Snippet = p.lineSnippet(perr.Line)
		}
		return nil, perr
	}
	return doc, nil
}

// MustParse 同 Parse，解析失败时 panic。用于测试与字面量初始化。
func MustParse(data []byte) *Document {
	doc, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return doc
}
