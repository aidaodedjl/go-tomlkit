# go-tomlkit 设计文档

日期：2026-11-21
状态：待用户批准

## 意图与动机

Go 生态没有保格式（round-trip fidelity）的 TOML 读写库。BurntSushi/toml 和
pelletier/go-toml v2 都只做「解析到结构再整体重序列化」，注释、空行、键序、
引号风格全部丢失。go-toml v2 官方明确不支持往返保真。Python 有 tomlkit，
Rust 有 toml-edit，Go 没有对应物。

两个真实消费方已经为这个空缺付过代价：

- `deploy_toolbox`（Go 后端）在 `backend/config.go` 手写了约 400 行的行级
  编辑器（tomlEditor），按表头切区块、未触及行原样透传、已知键行内替换。
  代价是能力上限：多行数组、内联表、多行字符串一律显式报错拒绝。
- `TestMotion` 有两套。Python 侧用 tomlkit 三行完成同样的事，
  `del doc["actions"]` 换整个表、`doc["play_audio"] = True` 改标量、
  `dumps` 写回，格式全保。Go 侧（`backend/tomlrw.go` 588 行加
  `editor.go` 468 行，2026-11-21 从 deploy_toolbox 搬运重构）是第二套
  手写行级编辑器，构造期拒绝 dotted keys、array of tables、多行字符串、
  CRLF。

本库直接替换两套手写编辑器，并把那些报错形态变成正常支持。

## 目标

1. TOML 1.0.0 全语法解析，含多行数组、内联表、多行字符串、dotted keys、
   array of tables、全部四种 datetime 形态。
2. AST 级保真文档树。每个条目保留格式信息（前导注释与空行、缩进、键的
   书写形态、值的原始文本、行尾注释）。
3. 编辑 API 双层：核心树 API 加点路径糖（用户已确认）。
4. 写回行为规范（下节详列），核心承诺是未触及内容字节级不变。
5. 通过 toml-test 官方套件（valid 全部解析等价，invalid 全部拒绝）。
6. round-trip identity：解析后不做任何修改再写回，输出与输入逐字节一致。
7. 认真填生态空缺的发布标准：API 稳定性承诺、零第三方运行时依赖、
   完整文档、CI。

## 非目标

- 格式化与重排版（tomlfmt 的生态位，另立项目）。
- TOML 1.1 draft 特性（如内联注释后的换行规则变化、多行内联表尾逗号
  宽松化）。1.0 站稳后再议。
- struct 绑定（Unmarshal 到用户类型）。BurntSushi 已覆盖该需求，本库提供
  `Any()` 桥接原生 Go 值即可。
- 流式解析与超大文件优化。目标场景是配置文件，全内存树。
- 并发安全。文档树与 tomlkit、BurntSushi 一样不做内部锁，并发访问由
  调用方串行化。

## 反目标（做到技术上能跑也算失败）

- 静默损坏文件。手写编辑器的「无法安全处理就报错，绝不静默降级」原则
  全库继承。
- 未触及内容发生任何字节变化。
- 语义漂移。写回结果再解析，必须与「原文件应用了同样的修改」语义等价。

## 约束

- Module path：`github.com/aidaodedjl/go-tomlkit`，package 名 `tomlkit`
  （用户已确认）。
- 纯 Go 标准库，零第三方运行时依赖。toml-test 套件仅作为测试依赖引入。
- Go 版本取当前稳定版。
- 对外 API 面在 v1 之前允许调整，v1 起按稳定承诺维护。

## 核心行为规范（写回语义）

这是本库的灵魂，逐条列出，测试逐条对照。

1. 解析后不做任何修改，写回输出与输入逐字节一致。
2. 修改已有标量：原位替换值部分。键的书写形态（裸键或引号键、引号
   风格）、行首缩进、行尾注释与空格全部保留。
3. 删除键或表：该条目连同其前导注释与空行一起移除，其余内容不变。
4. 新增键：追加到所在表的末尾（该表最后一个条目之后，不改已有行）。
5. 整表替换 `Set(key, table)`：表头行留在原位置（含其前导注释），区块
   内容整体替换为新表内容。这是我的判断，理由是用户配置文件里表的位置
   顺序是作者的表达，整体挪到文件末尾会破坏阅读结构。TestMotion 的
   `del + 重新赋值` 用法在此语义下得到等价结果。
6. 新增表：追加到文件末尾，前置一个空行。
7. 键序等于文件出现序。任何读操作不重排，`Keys()` 按出现序返回。
8. 新值渲染默认风格：string 用基本双引号串（必要转义）；integer 十进制；
   float 用 Go `strconv` 最短 round-trip 表示并保证再解析值不变
   （TOML 要求小数点前后都有数字，如 `1.0` 不写成 `1`）；bool 写
   `true`/`false`；datetime 按原 kind 渲染（offset datetime、local
   datetime、local date、local time 四种，见 API 草案的类型设计）。
9. 数组与内联表内的元素修改：保持原容器形态。多行数组改一个元素仍是
   多行数组，逐元素原位替换。
10. dotted key（如 `a.b = 1`）解析为嵌套表视图，同时保留「此处原本是
    dotted key 写法」的格式记录，未修改时原样写回。修改其内部键时，
    该行按修改后的值重写为 dotted key 形态。与显式 `[a]` 表头混用时
    遵循 TOML 1.0 定义表的语义（dotted key 定义的表不能再被 `[a]` 重复
    定义，反之亦然，解析期报错）。
11. 解析错误带位置信息（行、列、字节偏移）与出错行原文片段。
12. 文件开头的 UTF-8 BOM 接受并在写回时原样透传。BOM 出现在其他位置是
    解析错误。依据 toml-test 立场（`tests/valid/utf8-bom-*` 与
    `tests/invalid/encoding/bom-not-at-start-*`，2026-11-21 查证）。
    两个消费方现状是对 BOM 报错（对齐 tomllib），本库跟随 toml-test
    更宽容，不构成迁移障碍。
13. 换行风格。未触及行透传。渲染新增行时跟随文件主导换行风格（统计
    文件中 CRLF 与 LF 出现次数取多者，无换行或平局取 LF）。
14. 写回恒为合法 TOML 且语义等于「原文件应用了已做的修改」。用 property
    测试验证（见测试标准）。

## API 草案（用户已确认双层形状）

```go
package tomlkit

// 解析与写回
func Parse(data []byte) (*Document, error)
func MustParse(data []byte) *Document
func (d *Document) String() string // 树内状态恒合法，写回不失败
func (d *Document) Bytes() []byte

// 核心树 API
type Item interface {
    Type() ItemType // Table, InlineTable, Array, String, Integer, Float,
                    // Boolean, Datetime
}
// 类型化访问器，第二个返回值表示类型是否匹配
func (i Item) AsBool() (bool, bool)
func (i Item) AsInteger() (int64, bool)
func (i Item) AsFloat() (float64, bool)
func (i Item) AsString() (string, bool)
func (i Item) AsDatetime() (Datetime, bool)
func (i Item) Any() any // 转原生 Go 值（map[string]any / []any / 标量），丢弃格式

// 表导航，Get 未命中返回 nil
func (d *Document) Get(key string) Item
func (t *Table) Get(key string) Item
func (t *Table) Table() *Table        // receiver 为 Item 接口下的具体类型方法集
func (t *Table) Keys() []string       // 文件出现序

// 写入。Set 的值接受原生 Go 值（bool, int64, float64, string, Datetime,
// []any, map[string]any, Item）以及库提供的构造器产物。不可渲染的值
// 返回 error。
func (d *Document) Set(key string, v any) error
func (t *Table) Set(key string, v any) error
func (t *Table) Delete(key string) bool

// 路径糖。点分路径一行到位，语义与逐级 Get/Set 完全相同。
func (d *Document) GetPath(path string) Item        // "actions.wave.volume"
func (d *Document) SetPath(path string, v any) error
func (d *Document) DeletePath(path string) bool
// 键名本身含点或引号时的显式形式
func (d *Document) GetPathKeys(keys []string) Item
func (d *Document) SetPathKeys(keys []string, v any) error
func (d *Document) DeletePathKeys(keys []string) bool

// 从零构建
func NewDocument() *Document
func NewTable() *Table
func NewInlineTable() *InlineTable
func NewArray() *Array

// Datetime：四种 kind 用类型区分，time.Time 无法表达 local date 与
// local time，所以自带类型。
type Datetime struct { /* kind + time.Time 或字段 */ }
```

具体方法集分布（哪些方法挂在 `*Table`、哪些挂在 `Item` 接口）留给实现
计划阶段定，以上只锁定形状与语义。

## 架构方向

含我的判断，builder 可在计划阶段挑战但需给出理由。

1. 单层混合树（tomlkit 式），不做 toml-edit 式双层（格式 CST 与语义树
   分离）。理由是 Go 实现里单层树直接对应公开 API，层少一半，保真信息
   作为条目字段挂在树上。
2. 自写 lexer 加递归下降 parser。token 保留原始文本区间与行列位置。
   不 fork BurntSushi，它的 parser 不保留格式信息，改造成本高于重写。
3. 每个条目携带 decor 结构：前导 trivia（注释、空行、缩进）、键书写
   形态、值原始文本、行尾 trivia（注释、空白）。
4. 写回分两条路径。未修改条目直接拼接原始文本区间，保证字节一致。
   修改或新增条目走渲染器，渲染风格即行为规范第 8 条。
5. 包布局：`tomlkit/` 根包只放稳定 API（Document, Table, Item, 构造器，
   Parse），parser 与渲染器放 `internal/`，测试数据的黄金文件放
   `testdata/`。

## 测试标准

1. toml-test 套件（toml-lang/toml-test）。valid 用例解析输出规范化 JSON
   对照，invalid 用例全部拒绝。这是解析正确性的外部锚。
2. round-trip identity。全部 valid 用例加真实世界样本（Cargo.toml、
   pyproject.toml、deploy_toolbox 的 config.toml 形态），含带 BOM 与
   CRLF 的样本。解析后原样写回，字节一致。
3. 行为规范逐条测试。上面 14 条规范每条至少一个直接对照用例。
4. Property 测试。随机选取条目做随机修改，写回后重新解析，断言两点。
   修改的条目取得新值。未触及区域与原文逐字节相同。这用 toml-test 的
   valid 语料做种子语料。
5. 集成场景测试，两个消费方各自的形态。
   deploy_toolbox `config.go` 的真实场景：换整个 actions 表、改标量、
   保缩进与行尾注释、新键按序追加。
   TestMotion Go 侧 `tomlrw.go` 的行为清单对齐进验收（用户已确认）：
   actions 保文件序（`Keys()` 按出现序）、四种行级操作语义
   （原位换值、表尾插键、整表追加、区间删除）各有直接用例，其真实
   配置形态脱敏后进 testdata。Python 侧 `toml_handler.py` 的三行操作
   场景同。

## 里程碑

给实现计划展开用，顺序即交付顺序，每个里程碑以可验证检查收尾。

- M1 lexer 与 parser，TOML 1.0.0 全语法，toml-test valid 与 invalid
  全过。
- M2 文档树与写回，round-trip identity 全过。
- M3 编辑 API（树与路径双层）与修改渲染，行为规范 14 条测试全过，
  property 测试接入。
- M4 从零构建 API、错误信息打磨、README、CI、发布准备。

## 留给 builder 的问题

以下不阻塞设计批准，计划阶段自行决定并记录。

- Item 接口与具体类型的方法集精确分布。
- Array 元素的编辑 API 形状（索引 Get/Set/Append/长度）。
- decor 结构的内部字段精确划分。
- toml-test 的引入方式（go module 依赖还是 vendored 子模块）。
- CI 平台与矩阵（GitHub Actions 起步即可）。

## 决策记录

用户确认项：

- 标准为「认真填生态空缺」，全 TOML 1.0 语法、toml-test 套件、API 稳定
  承诺。
- API 形状为双层，核心树 API 加点路径糖。
- 名字 go-tomlkit，module path `github.com/aidaodedjl/go-tomlkit`，
  GitHub 账号 aidaodedjl。
- 主要场景是读用户配置、程序改几项设置后保存。消费方为 deploy_toolbox
  与 TestMotion（Python 侧加 Go 侧两套手写编辑器，Go 侧 tomlrw.go 的
  行为清单对齐进验收）。

我的判断项（用户可推翻）：

- 整表替换保留表头原位置（行为规范第 5 条）。
- 单层混合树而非双层 CST（架构方向第 1 条）。
- 自写 parser 而非 fork（架构方向第 2 条）。
- Datetime 自带四 kind 类型而非裸 time.Time（API 草案）。
- 零第三方运行时依赖。
- 新增表前置一个空行（行为规范第 6 条）。
- BOM 开头接受并透传，非开头报错（行为规范第 12 条，依据 toml-test
  立场查证）。
- 新增行换行风格跟随文件主导风格（行为规范第 13 条）。
- README 英文正文（Go 社区惯例），另附 `README.zh-CN.md` 互链（M4）。
