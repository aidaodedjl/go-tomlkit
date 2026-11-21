package tomlkit_test

import (
	"fmt"

	tomlkit "github.com/aidaodedjl/go-tomlkit"
)

// 原位编辑：保留注释、缩进与键序。
func Example_editInPlace() {
	doc := tomlkit.MustParse([]byte("volume = 0.8  # keep me\n"))
	if err := doc.Set("volume", 1.0); err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", doc.String())
	// Output: "volume = 1.0  # keep me\n"
}

// 点路径糖与类型化访问。
func Example_dotPath() {
	doc := tomlkit.MustParse([]byte("[actions.wave]\nvolume = 0.8\n"))
	v, _ := doc.GetPath("actions.wave.volume").AsFloat()
	fmt.Println(v)
	if err := doc.SetPath("actions.wave.volume", 0.5); err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", doc.String())
	// Output:
	// 0.8
	// "[actions.wave]\nvolume = 0.5\n"
}

// 整表替换：表头留在原位，区块内容整体换新。
func Example_replaceTable() {
	doc := tomlkit.MustParse([]byte("# actions\n[actions]\nold = true\n[after]\nk = 1\n"))
	actions := tomlkit.NewTable()
	wave := tomlkit.NewTable()
	_ = wave.Set("name", "wave")
	_ = actions.Set("wave", wave)
	if err := doc.Set("actions", actions); err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", doc.String())
	// Output: "# actions\n[actions]\n\n[actions.wave]\nname = \"wave\"\n[after]\nk = 1\n"
}

// 删除键：连同前导注释一起移除，其余不变。
func Example_delete() {
	doc := tomlkit.MustParse([]byte("# lead\ndrop = 1\nkeep = 2\n"))
	doc.Delete("drop")
	fmt.Printf("%q\n", doc.String())
	// Output: "keep = 2\n"
}

// 从零构建并写回。
func Example_buildFromScratch() {
	doc := tomlkit.NewDocument()
	_ = doc.Set("title", "demo")
	tbl := tomlkit.NewTable()
	_ = tbl.Set("x", 1)
	_ = doc.Set("tbl", tbl)
	fmt.Printf("%q\n", doc.String())
	// Output: "title = \"demo\"\n\n[tbl]\nx = 1\n"
}

// 解析错误带位置与原文片段。
func Example_parseError() {
	_, err := tomlkit.Parse([]byte("a = 1\nbad =\n"))
	fmt.Println(err)
	// Output: tomlkit: line 2, col 6: expected value after '=': "bad ="
}

// 解析错误的多行展示（带列指示）。
func ExampleParseError_detailed() {
	_, err := tomlkit.Parse([]byte("a = 1\nbad =\n"))
	fmt.Println(err.(*tomlkit.ParseError).Detailed())
	// Output:
	// tomlkit: expected value after '='
	//     bad =
	//          ^
}
