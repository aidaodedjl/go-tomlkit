# go-tomlkit（中文）

Go 的保格式（round-trip fidelity）TOML 编辑库——Python
[tomlkit](https://github.com/sdispather/tomlkit) 与 Rust
[toml-edit](https://github.com/toml-rs/toml-edit) 在 Go 生态的对应物。

BurntSushi/toml 与 pelletier/go-toml 把 TOML 解析成值再整体重序列化：
注释、空行、键序、引号风格全部丢失。go-tomlkit 解析成保留每个格式字节的
文档树，原位编辑值，写回时**未修改的部分逐字节不变**。

```go
doc, err := tomlkit.Parse(data)
if err != nil { ... }
if err := doc.Set("play_audio", true); err != nil { ... }
if err := os.WriteFile(path, doc.Bytes(), 0o644); err != nil { ... }
```

## 安装

```sh
go get github.com/aidaodedjl/go-tomlkit
```

零第三方运行时依赖。TOML 1.0.0。

## 承诺

- 不做修改就 `String()`，输出与输入**逐字节一致**（以
  [toml-test](https://github.com/toml-lang/toml-test) valid 全集验证，
  含 BOM 与 CRLF 文件）。
- 替换值保留键的书写形态、缩进与行尾注释。
- 追加键落在该表键区的末尾，绝不出现在子表头之后（那会改变键的归属）。
- 删除键连同其前导注释一起移除，其余内容不变。
- 写回结果重新解析的语义恒等于「原文件 + 你的修改」（1000 轮随机编辑
  property 测试验证）。
- 非法 TOML 带行/列/原文片段报错拒绝，绝不静默损坏。

非目标：重排版、TOML 1.1 draft 特性、struct 绑定（BurntSushi 已覆盖）、
并发安全。路径糖不穿透表数组（[[x]]）：用 `Get("x").Array()` 导航后
以元素级 API（`Set(i, *Table)` / `Append(*Table)`）编辑。

## 用法

完整示例见 [README.md](README.md)（英文）：

- 原位编辑：`doc.Set("volume", 1.0)` 保注释
- 点路径糖：`doc.GetPath / SetPath / DeletePath`，含点键名用 `*PathKeys`
- 类型化访问：`it.AsBool() / AsFloat() / Any()`
- 整表替换：`doc.Set("actions", tomlkit.NewTable())` 表头留在原位
- 从零构建：`tomlkit.NewDocument()`

## 与 BurntSushi/toml 的分工

解码到 struct、从零重写整个文件，用 BurntSushi。
文件属于人类、改完必须原样还回去，用 go-tomlkit。

## 许可

MIT。
