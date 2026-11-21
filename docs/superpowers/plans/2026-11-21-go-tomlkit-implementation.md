# go-tomlkit 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Go 生态的 tomlkit 等价物，保格式（round-trip fidelity）TOML 读写库。

**Architecture:** 单包布局。lexer 与递归下降 parser 是根包未导出文件，解析成带格式信息（decor）的单层混合文档树。写回双路径：未修改条目拼接原始文本区间保证字节一致，修改条目走默认风格渲染器。编辑 API 双层：树 API 加点路径糖。

**Tech Stack:** 纯 Go 标准库，零第三方运行时依赖。toml-test（github.com/toml-lang/toml-test）仅作测试依赖。

**Spec:** `docs/superpowers/specs/2026-11-21-go-tomlkit-design.md`

**对 spec 的一处偏离（已声明）：** spec 架构方向第 5 条说 parser 放 `internal/`。本计划改为单包未导出文件（`lexer.go`、`parser.go`），理由是 parser 需直接填充树类型的未导出字段，跨包会迫使导出内部构造函数。对外 API 面不受影响，spec 的意图（稳定 API 只在根包）通过导出符号控制实现。

## Global Constraints

- Module `github.com/aidaodedjl/go-tomlkit`，package `tomlkit`。
- 零第三方运行时依赖。`go.mod` 的 require 里只允许出现 `github.com/toml-lang/toml-test`（indirect 也查）。
- `go.mod` 声明执行机当前稳定 Go 版本。
- git 只 commit 不 push。push 与 PR 创建需用户审批。
- commit message 纯净，禁止 Co-Authored-By、Made-with 等尾注。
- 每个任务结束时 `go test ./...` 与 `go vet ./...` 全绿才能 commit。
- 行为规范 14 条的编号引用 spec 文档，实现与测试逐条对照。

## Review Focus

spec 是愿景文档，以下是它没有点名、但真实输入会咬人的五类，各 pin 到所属任务。

1. 空文件与纯注释文件。Parse 必须成功，String() 与输入逐字节一致，含无尾换行、只有注释、CRLF 文件。测试在 Task 8。
2. 重复定义。`a=1` 后再 `[a]`、表头两次、kv 重复键，全部解析报错带行号，不是静默覆盖。测试在 Task 4 与 Task 14。
3. 未闭合结构。未闭合多行字符串、多行数组、内联表必须报错，不能死循环或 panic。测试在 Task 5。
4. 数值边界。int64 溢出报错；inf/nan 渲染为 TOML 合法文本（`inf`、`+inf`、`-inf`、`nan`）；float 渲染满足 parse(render(x)) == x。测试在 Task 11。
5. 深嵌套。超长 dotted key 路径（1000 段）与深层 AoT 不栈溢出不 panic。测试在 Task 5。

---

## 文件结构

```
tomlkit-go/
├── go.mod
├── tomlkit.go    # Parse/MustParse/Document/String/Bytes
├── item.go       # Item 接口、ItemType、类型化访问器
├── decor.go      # decor 结构（格式信息）
├── value.go      # 标量节点：String/Integer/Float/Boolean
├── datetime.go   # Datetime 四 kind
├── table.go      # Table、InlineTable
├── array.go      # Array（单行/多行、AoT）
├── lexer.go      # tokenizer（未导出）
├── parser.go     # 递归下降（未导出）
├── render.go     # 写回拼接与值渲染（未导出核心）
├── path.go       # 路径糖
├── builder.go    # NewDocument/NewTable/NewInlineTable/NewArray
├── error.go      # ParseError
├── testdata/     # 黄金文件（含 BOM/CRLF/无尾换行样本、消费方脱敏样本）
├── .github/workflows/ci.yml
└── README.md、README.zh-CN.md
```

## 核心类型（跨任务契约，所有任务按此引用）

```go
// decor.go — 每个条目与表头的格式信息
type decor struct {
    prefix string // 上一条目结束到键名（或表头 [）之间的一切原文：空行、注释、缩进
    keyRaw string // 键（或表头路径）的原文书写，含引号与点
    sep    string // kv 分隔的原文，如 "="、" = "、"  =  "
    tail   string // 值结束到行尾的原文：空白与行尾注释
    eol    string // "\n"、"\r\n" 或 ""（文件末行无换行）
    dotted bool   // 该条目源自己点写法 a.b = 1
}

// item.go
type ItemType int8
const (
    ItemTable ItemType = iota
    ItemInlineTable
    ItemArray
    ItemString
    ItemInteger
    ItemFloat
    ItemBoolean
    ItemDatetime
)

type Item interface {
    Type() ItemType
    AsBool() (bool, bool)
    AsInteger() (int64, bool)
    AsFloat() (float64, bool)
    AsString() (string, bool)
    AsDatetime() (Datetime, bool)
    Any() any // 原生 Go 值：map[string]any / []any / bool / int64 / float64 / string / Datetime
    Table() (*Table, bool)
    InlineTable() (*InlineTable, bool)
    Array() (*Array, bool)
}

// table.go
type Table struct {
    d         decor
    keys      []string          // 出现序
    items     map[string]Item   // 键为解码后的字符串
    implicit  bool              // dotted key 中间节点或被 [a.b] 隐式创建
    hasHeader bool              // 有显式 [x] 表头行
}

// value.go — 标量节点共享形状：decor + Go 值 + raw（原文）
type String  struct { d decor; v string;   raw string }
type Integer struct { d decor; v int64;    raw string }
type Float   struct { d decor; v float64;  raw string }
type Boolean struct { d decor; v bool;     raw string }
// raw == "" 表示已被修改，写回走渲染器；raw != "" 写回直接用 raw。

// datetime.go
type DatetimeKind int8
const (
    KindOffsetDatetime DatetimeKind = iota // 1979-05-27T07:32:00Z
    KindLocalDatetime                      // 1979-05-27T07:32:00
    KindLocalDate                          // 1979-05-27
    KindLocalTime                          // 07:32:00
)
type Datetime struct {
    Kind DatetimeKind
    Time time.Time // local date/time 用零日期承载时钟字段
    d    decor
    raw  string
}

// tomlkit.go
type Document struct {
    bom  []byte // 开头 BOM 原文，无则 nil
    root *Table
}
func Parse(data []byte) (*Document, error)
func MustParse(data []byte) *Document
func (d *Document) String() string
func (d *Document) Bytes() []byte

// error.go
type ParseError struct {
    Line, Col int // 1-based
    Offset     int // 0-based 字节偏移
    Snippet    string // 出错行原文
    Msg        string
}
func (e *ParseError) Error() string
```

`Table` 的写入方法、`Array` 内部结构、路径糖签名在各任务定义。

---

## 里程碑 M1：lexer 与 parser

### Task 1: 项目脚手架与冒烟测试

**Files:**
- Create: `go.mod`、`tomlkit.go`、`tomlkit_test.go`

**Interfaces:**
- Produces: `Parse(data []byte) (*Document, error)`、`MustParse`、`Document` 最小骨架（先返回空文档也行，本任务只验证包结构可编译可测试）。

- [ ] **Step 1: 初始化 module**

Run: `go mod init github.com/aidaodedjl/go-tomlkit`
写 `tomlkit.go`：`Document` 结构（bom、root 字段占位）、`Parse` 返回 `nil, errors.New("not implemented")`、`MustParse` panic 包装。

- [ ] **Step 2: 写失败测试**

```go
func TestParseSmoke(t *testing.T) {
    _, err := Parse([]byte("a = 1\n"))
    if err == nil { t.Fatal("expected error until implemented") }
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./...`
Expected: FAIL。

- [ ] **Step 4: Commit**

```bash
git add go.mod tomlkit.go tomlkit_test.go
git commit -m "feat: module 骨架与 Parse 入口"
```

### Task 2: lexer 基础（行结构与 trivia）

**Files:**
- Create: `lexer.go`、`lexer_test.go`

**Interfaces:**
- Produces（包内未导出，Task 4 消费）:

```go
type tokenKind int8
const (
    tEOF tokenKind = iota
    tNewline        // \n 或 \r\n
    tComment        // # 到行尾（不含换行）
    tWhitespace     // 行内空白
    tKey            // 裸键或引号键（解码后的字符串值 + raw 原文）
    tEquals         // =
    tTableHeader    // [path] 或 [[path]]，raw 原文 + path 段
    tValue          // 单个值 token：字符串/数字/bool/datetime/内联表/数组 的原文起点
)
type token struct {
    kind  tokenKind
    raw   string // 原文
    line  int    // 1-based
    col   int    // 1-based
    off   int    // 0-based 字节偏移
    // tKey/tTableHeader 时填充
    str   string   // 解码后键名
    parts []keyPart // 表头路径段
}
type keyPart struct{ name string; raw string }
func lex(data []byte) ([]token, *ParseError)
```

lexer 对值 token 不做完整解析（内联表、数组的开括号出现即归入值 token 的起点，值域切分交给 Task 3 的值扫描器），本任务先交付：裸键/引号键（基本串与字面串）、`=`、表头两种形态、注释、换行（LF 与 CRLF）、行内空白、BOM 识别（开头 BOM 记录并跳过，非开头 BOM 报错）。

- [ ] **Step 1: 写失败测试**（`lexer_test.go`）

至少覆盖：`a = 1` 的 token 序列（tKey, tWhitespace, tEquals, tWhitespace, tValue）；`[a.b]` 表头 parts；CRLF 换行 token；开头 BOM 被剥离并记录；非开头 BOM 报错带行列；`# comment` 归 tComment。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test -run TestLex ./...`
Expected: FAIL（lex 未定义）。

- [ ] **Step 3: 实现 `lex`**

逐字节扫描。键的引号形态与转义按 TOML 1.0 规范。表头路径按 `.` 切分，每段可以是裸键或引号键。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test -run TestLex ./...`
Expected: PASS。

- [ ] **Step 5: Commit** `feat: lexer 基础（键/表头/trivia/BOM）`

### Task 3: 值扫描器（全部标量与复合值 token 化）

**Files:**
- Modify: `lexer.go`
- Test: `lexer_test.go`

**Interfaces:**
- Produces: `lex` 输出的 tValue token 能覆盖完整值域。值内不再产 token，值 token 的 raw 覆盖值全文（复合值到匹配的闭合括号）。
- 值形态枚举（供 parser 分类）:

```go
type valueKind int8
const (
    vString valueKind = iota // 三种：基本串 "..."、字面串 '...'、多行 """...""" / '''...'''
    vInteger                 // 十进制/0x/0o/0b/下划线
    vFloat                   // 含 inf/nan/+/-号
    vBoolean                 // true/false
    vDatetime                // 四 kind
    vInlineTable             // {...}
    vArray                   // [...] 含跨行
)
```

- [ ] **Step 1: 写失败测试**

覆盖：基本串含全部转义（`\uXXXX`、`\UXXXXXXXX`、`\b\t\n\f\r\"\\`）；字面串；多行基本串（首行换行裁剪、行尾反斜杠续行）与多行字面串；整数四种进制与下划线；float 小数与指数形态、`inf`/`nan`/`+inf`；datetime 四 kind 原文；内联表嵌套；数组嵌套与跨行（含注释与尾逗号）；未闭合引号/括号报错带行列（Review Focus 3）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test -run TestLexValue ./...`
Expected: FAIL。

- [ ] **Step 3: 实现值扫描**

字符串扫描器处理转义与多行裁剪规则。括号匹配用计数器跨行扫描数组与内联表，内部允许注释与换行。

- [ ] **Step 4: 跑测试确认通过后 Commit** `feat: 值扫描器（全值域 token 化）`

### Task 4: ParseError 与 parser 骨架（根表、kv、表头）

**Files:**
- Create: `error.go`、`parser.go`、`parser_test.go`

**Interfaces:**
- Consumes: `lex`、`token`。
- Produces: `parse(data []byte) (*Document, *ParseError)`（包内未导出，`Parse` 调它）；填充 `Table`/`String`/`Integer`/`Float`/`Boolean` 节点与 decor；`Document.bom`。

- [ ] **Step 1: 写失败测试**

```go
func TestParseBasic(t *testing.T) {
    doc, err := Parse([]byte("a = 1\nb = \"x\"\n"))
    // 断言根表 keys 顺序 ["a","b"]
    // doc.Get 尚未实现，直接检查内部 root（包内测试）
}
func TestParseTableHeader(t *testing.T) {
    // [a]\nx = 1\n[a.b]\ny = 2\n → root 含 a，a 含 x 与子表 b
}
func TestParseDuplicate(t *testing.T) {
    // "a = 1\na = 2\n" 报错；"[a]\nx=1\n[a]\ny=2\n" 报错；"a=1\n[a]\n" 报错（Review Focus 2）
    // 错误的 Line/Col/Snippet 正确
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test -run TestParse ./...`
Expected: FAIL。

- [ ] **Step 3: 实现 parser**

状态机两态：根区与当前表。表头切换当前表，kv 追加进当前表。decor 从 token 间原文重组：prefix 取上一 token 结束到键开始的字节区间，eol 取换行 token 原文。值按 valueKind 分派到标量构造（字符串解码转义、整数 strconv.ParseInt 前去下划线换进制、float 同理、bool）。

- [ ] **Step 4: 跑测试确认通过后 Commit** `feat: parser 骨架与重复定义检测`

### Task 5: 完整语法（dotted keys、内联表、数组、AoT、datetime）

**Files:**
- Modify: `parser.go`、`datetime.go`
- Test: `parser_test.go`

**Interfaces:**
- Consumes: Task 3 的复合值 token、Task 4 的 parser。
- Produces: `Array`（含 `items []Item`、逐元素 decor、`multiline bool`、`raw string`）、`InlineTable`（同 Table 结构但无表头语义）、dotted key 展开为嵌套 implicit Table、`Datetime` 四 kind 解析。
- `Array` 与 `InlineTable` 满足 `Item` 接口的定义在本任务补齐方法集。

- [ ] **Step 1: 写失败测试**

dotted key `a.b.c = 1` 生成 implicit 链且 `dotted` 标记在叶子；`[a]` 与 `a.b = 1` 混用冲突报错；内联表嵌套与内联表内 dotted key；多行数组保元素顺序与内部注释；AoT `[[a]]` 两次生成 Array of Table；datetime 四 kind 的 Kind 字段与 Time 值正确；深嵌套 1000 段 dotted key 不 panic（Review Focus 5）；int64 溢出报错。

- [ ] **Step 2: 跑测试确认失败后 Step 3 实现**

datetime 解析格式串依次尝试：`time.RFC3339Nano`、`2006-01-02T15:04:05.999999999`、`2006-01-02 15:04:05.999999999`、`2006-01-02`、`15:04:05.999999999`。数组的逐元素 decor：元素间原文（空白、注释、换行）归前元素 tail 或后元素 prefix，任选一致约定并写注释。

- [ ] **Step 4: 全量 parser 测试通过后 Commit** `feat: TOML 1.0 全语法解析`

### Task 6: toml-test 接入

**Files:**
- Create: `tomltest_test.go`
- Modify: `go.mod`（测试依赖）

**Interfaces:**
- Produces: CI 可跑的 toml-test 全量门禁。

- [ ] **Step 1: 引入依赖**

Run: `go get github.com/toml-lang/toml-test@latest`
若该 module 不可编程引用，退路是 clone 到 `testdata/toml-test`（进 `.gitignore`），测试驱动扫描目录。二选一在执行时定，plan 不锁定。

- [ ] **Step 2: 写 runner 测试**

实现两个入口的适配器：`Parse` + 值导出到 toml-test 期望的 JSON 形态（表、数组、类型标签 `{"type":"integer","value":"1"}`）。valid 全集断言 JSON 等价；invalid 全集断言返回 error。

- [ ] **Step 3: 跑全量，修复到全绿**

Run: `go test -run TestTomlTest ./... -v`
Expected: valid 与 invalid 全过。修不动的个别用例（如 toml-test 版本超前）记录在测试文件头注释，不许静默 skip。

- [ ] **Step 4: Commit** `test: toml-test valid/invalid 全量门禁`

---

## 里程碑 M2：写回与 round-trip identity

### Task 7: Document.String() 写回

**Files:**
- Create: `render.go`、`render_test.go`

**Interfaces:**
- Consumes: 全部树类型与 decor。
- Produces: `Document.String()`、`Document.Bytes()`；未导出 `renderValue(v any) string`（默认风格渲染器，Task 11 用）。

- [ ] **Step 1: 写失败测试**

解析 → 不修改 → String() 与输入逐字节一致。用例集：单行 kv、带注释、带空行、CRLF、无尾换行、BOM 文件、多行数组、多行字符串、内联表、AoT、dotted key。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test -run TestRoundTrip ./...`
Expected: FAIL。

- [ ] **Step 3: 实现写回**

递归拼接：Table 输出 `prefix + [keyRaw] + tail + eol` 再依次输出子条目；kv 输出 `prefix + keyRaw + sep + (raw 非空 ? raw : renderValue) + tail + eol`；Array 多行时逐元素带内部 decor；Document 先输出 bom。`renderValue` 本任务先实现标量默认风格（string 双引号转义、int 十进制、float 最短且保证 `1.0` 不渲染成 `1`、bool、datetime 按 kind），复合值渲染 Task 11 补。

- [ ] **Step 4: 跑测试确认通过后 Commit** `feat: 写回与原始字节拼接`

### Task 8: round-trip identity 全量门禁

**Files:**
- Create: `testdata/`（BOM、CRLF、无尾换行、真实样本）、`roundtrip_test.go`

**Interfaces:**
- Consumes: Task 6 的 toml-test 语料、Task 7 的 String()。
- Produces: 遍历 toml-test valid 全集 + testdata 样本的 identity 断言。

- [ ] **Step 1: 采集 testdata**

BOM 开头文件、CRLF 文件、无尾换行文件、纯注释文件、空文件（Review Focus 1）。从 `/Users/djl/JuShen/deploy_toolbox/backend/` 与 `/Users/djl/JuShen/TestMotion/backend/` 采集真实 config.toml 形态，脱敏（改主机名、IP、路径中的专有名）后存入。

- [ ] **Step 2: 写测试**

遍历全部 toml-test valid `.toml` 与 `testdata/**/*.toml`：`Parse(b)` 无错，且 `[]byte(doc.String())` 与 `b` 逐字节相等。

- [ ] **Step 3: 跑全量修复到绿后 Commit** `test: round-trip identity 全量门禁`

---

## 里程碑 M3：编辑 API

### Task 9: Item 接口与类型化访问器

**Files:**
- Create: `item.go`、`item_test.go`

**Interfaces:**
- Produces: `Item` 接口全方法集（见「核心类型」）在八个具体类型（Table、InlineTable、Array、String、Integer、Float、Boolean、Datetime）上实现；`Any()` 递归转换（Table → `map[string]any` 按出现序无序，值递归；Array → `[]any`）。

- [ ] **Step 1: 写失败测试**：每种类型的 AsXxx 命中与不命中（String.AsInteger 返回 false）；Any() 对嵌套文档的完整转换。
- [ ] **Step 2: 跑测试确认失败后 Step 3 实现**：类型不匹配统一返回零值 false。
- [ ] **Step 4: 通过后 Commit** `feat: Item 接口与访问器`

### Task 10: 读导航与删除

**Files:**
- Create/Modify: `table.go`、`table_test.go`

**Interfaces:**
- Produces:

```go
func (d *Document) Get(key string) Item      // 根表 Get，未命中 nil
func (t *Table) Get(key string) Item
func (d *Document) Keys() []string
func (t *Table) Keys() []string              // 出现序
func (d *Document) Delete(key string) bool
func (t *Table) Delete(key string) bool      // 连带前导注释（spec 行为规范 3）；表删除删整区块
```

- [ ] **Step 1: 写失败测试**：Get 嵌套取值；Keys 出现序（含 AoT 与 dotted 展开）；Delete 标量后 String() 中该行与其前导注释消失、其余字节不变；Delete 表删整区块（含其全部子表，对齐 tomlrw.go 区间删除语义）。
- [ ] **Step 2: 确认失败后 Step 3 实现**：删除 = 从 keys 与 items 移除，写回自然消失（无墓碑）。
- [ ] **Step 4: 通过后 Commit** `feat: 导航与删除`

### Task 11: Set 语义与值渲染器

**Files:**
- Create/Modify: `table.go`、`render.go`、`set_test.go`

**Interfaces:**
- Produces:

```go
func (d *Document) Set(key string, v any) error
func (t *Table) Set(key string, v any) error
```

`v` 接受 `bool`、`int`、`int64`、`float64`、`string`、`Datetime`、`[]any`、`map[string]any`、`Item`。其余类型（含 nil、chan、struct）返回 error。

- [ ] **Step 1: 写失败测试**

命中已有键：原位替换，prefix/keyRaw/sep/tail/eol 原样保留（`  a = 1  # note` 改值后缩进与注释不变，spec 规范 2）。未命中：表尾追加 `key = value`，eol 跟随文件主导风格，不插空行（规范 4、13）。渲染断言：`renderValue(1.0) == "1.0"`、`renderValue(0.1) == "0.1"`、inf/nan 四种文本、随机 float 满足 parse(render(x)) == x（Review Focus 4）、string 控制字符转义、int64 边界。`[]any`/`map[string]any` 渲染为单行数组与内联表。修改多行数组内单元素：数组保持多行形态，其余元素原文不变（规范 9）。
- [ ] **Step 2: 确认失败后 Step 3 实现**

float 渲染：`strconv.FormatFloat(f, 'g', -1, 64)` 后处理补 `.0`（无 `.` 与 `e` 且非 inf/nan 时）；inf/nan 特判 `+inf`/`-inf`/`inf`/`nan`。
- [ ] **Step 4: 通过后 Commit** `feat: Set 与默认渲染`

### Task 12: 整表替换与新表追加

**Files:**
- Modify: `table.go`、`render.go`
- Test: `set_test.go`

**Interfaces:**
- Consumes: Task 11 的 Set。
- Produces: `Set(key, *Table)` 语义：表头行留原位（含前导注释），区块内容整体替换（spec 规范 5）。新键赋 `*Table`（无原表）：文件末尾追加表，前置一个空行（规范 6）。

- [ ] **Step 1: 写失败测试**：解析含 `[actions]` 多子表的文件，Set("actions", NewTable()+新键)，输出中 `[actions]` 仍在原行、旧子表区块消失、新键在区块内；末尾追加新表带前置空行；对齐 TestMotion 场景 `del doc["actions"]; doc["actions"] = 新表` 的等价性。
- [ ] **Step 2: 确认失败后 Step 3 实现**：替换 = 原表节点保留 d 与 hasHeader，清空 keys/items 填新内容；dotted 子树与 implicit 链的表头路径重写为 keyRaw。
- [ ] **Step 4: 通过后 Commit** `feat: 整表替换与末尾追加`

### Task 13: 点路径糖

**Files:**
- Create: `path.go`、`path_test.go`

**Interfaces:**
- Produces:

```go
func (d *Document) GetPath(path string) Item            // "a.b.c" 逐级导航，未命中 nil
func (d *Document) SetPath(path string, v any) error    // 中间不存在自动建 implicit 表
func (d *Document) DeletePath(path string) bool
func (d *Document) GetPathKeys(keys []string) Item
func (d *Document) SetPathKeys(keys []string, v any) error
func (d *Document) DeletePathKeys(keys []string) bool
```

- [ ] **Step 1: 写失败测试**：字符串路径按 `.` 切分；`SetPath` 中间建表后写回生成 dotted key 形态行（规范 10）；键名含点的引号键用 Keys 变体；语义与逐级 Get/Set 一致性对照测试。
- [ ] **Step 2: 确认失败后 Step 3 实现**：切分只认字面 `.`，不做 TOML 引号解析（引号键走 Keys 变体），doc 注释写明。
- [ ] **Step 4: 通过后 Commit** `feat: 点路径糖`

### Task 14: 行为规范 14 条逐条收口

**Files:**
- Create: `spec_test.go`

**Interfaces:**
- Consumes: 全部 API。
- Produces: spec 行为规范 1 到 14 每条至少一个直接对照用例，用例名含规范编号。

- [ ] **Step 1: 写测试表**：14 条逐条，输入文件 + 操作 + 期望输出写在测试内联或 testdata。
- [ ] **Step 2: 跑全量**，有失败的回对应任务修（本任务不实现新功能，只暴露缺口）。Review Focus 2 的重复定义用例在此补全形态矩阵。
- [ ] **Step 3: Commit** `test: 行为规范 14 条对照测试`

### Task 15: Property 测试

**Files:**
- Create: `property_test.go`

**Interfaces:**
- Consumes: toml-test valid 语料、Set/Get/String。
- Produces: 随机编辑不变量门禁。

- [ ] **Step 1: 写测试**

种子随机（`math/rand` 固定种子保证可复现）。每轮：随机选 toml-test valid 文件 → 随机选一条根键路径 → 随机 Set 新值 → `String()` 后重新 `Parse`。断言三件事。目标路径读回新值。重新 Parse 无错。输出与原文按行 diff，变更行集合 ⊆ 目标条目行 ∪ 追加位置（规范 14 的机器验证）。

- [ ] **Step 2: 跑 1000 轮**，暴露的破坏回对应任务修复。

Run: `go test -run TestProperty ./...`
Expected: PASS。

- [ ] **Step 3: Commit** `test: 随机编辑 property 门禁`

### Task 16: 消费方集成场景

**Files:**
- Create: `integration_test.go`、`testdata/consumer/`

**Interfaces:**
- Consumes: 全部 API。
- Produces: 两个消费方行为清单的验收用例。

- [ ] **Step 1: 写测试**

deploy_toolbox `config.go` 场景：换整个 actions 表、改标量保缩进与行尾注释、新键按序追加（对照其测试期望的输出形态）。TestMotion `tomlrw.go` 行为清单：actions 保文件序（`Keys()` 断言）、原位换值、表尾插键、整表追加、区间删除各一用例。TestMotion Python 侧 `toml_handler.py` 三行操作等价用例。测试内嵌期望输出为黄金字符串。

- [ ] **Step 2: 跑通后 Commit** `test: 消费方集成场景验收`

---

## 里程碑 M4：从零构建与发布

### Task 17: 从零构建 API

**Files:**
- Create: `builder.go`、`builder_test.go`

**Interfaces:**
- Produces:

```go
func NewDocument() *Document
func NewTable() *Table
func NewInlineTable() *InlineTable
func NewArray() *Array
```

配合 `Set` 从空文档构建完整 TOML 并 String() 输出。

- [ ] **Step 1: 写失败测试**：从零构建 TestMotion 无原文分支的文档（`play_audio` + actions 表），输出为合法 TOML 且再 Parse 语义等价；空 Document String() 返回 ""。
- [ ] **Step 2: 确认失败后 Step 3 实现**：新建节点 raw 为空走渲染，表默认 eol 用 "\n"。
- [ ] **Step 4: 通过后 Commit** `feat: 从零构建 API`

### Task 18: README 与 CI

**Files:**
- Create: `README.md`、`README.zh-CN.md`、`.github/workflows/ci.yml`

**Interfaces:**
- Produces: 双语 README 互链；CI 跑 `go vet`、`go test`（含 toml-test 与 property 门禁）。

- [ ] **Step 1: 写 README**（英文正文）：一段定位（保格式读写，tomlkit/toml-edit 的 Go 对应物）、安装、5 个渐进示例（读改写、点路径、类型访问、从零构建、与 BurntSushi 分工）、round-trip 承诺与限制（1.0、非并发安全）。中文版对译。
- [ ] **Step 2: 写 CI**：push/pull_request 触发，`go test ./... -race` 加 vet。README 徽章占位。
- [ ] **Step 3: 本地全量验证后 Commit** `docs: README 与 CI`

### Task 19: 发布前收口

**Files:**
- Modify: 全仓

- [ ] **Step 1: 全量门禁**：`gofmt -l .` 空、`go vet ./...`、`go test ./... -race -count=1` 全绿。
- [ ] **Step 2: API 面审视**：`go doc` 过一遍导出符号，确认无内部泄漏（lexer/parser 的未导出符号不出现在文档）。
- [ ] **Step 3: Example 函数**：README 五个示例同步为 `example_test.go` 的 Example（godoc 可跑）。
- [ ] **Step 4: Commit** `chore: 发布前收口`。tag 与 push 留给用户审批，不执行。
