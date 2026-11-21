package tomlkit

// 标量节点。raw 是值的原文书写：raw != "" 时写回直接用原文，
// raw == "" 表示已被修改，写回走默认风格渲染器。

// String 是 TOML 字符串值。
type String struct {
	d   decor
	v   string
	raw string
}

// Integer 是 TOML 整数值（int64）。
type Integer struct {
	d   decor
	v   int64
	raw string
}

// Float 是 TOML 浮点值。
type Float struct {
	d   decor
	v   float64
	raw string
}

// Boolean 是 TOML 布尔值。
type Boolean struct {
	d   decor
	v   bool
	raw string
}
