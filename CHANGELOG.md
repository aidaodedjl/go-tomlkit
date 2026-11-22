# Changelog

## 0.1.2 (2026-11-22)

- fix：空容器挂子表取父表自身行位为 maxSeq 基线（T7）——挂载后的空壳超表没有条目可扫，
  基线缺失会把新表头排到行流前部、与既有 kv 平局错位（语义破坏）；解析态空表同此

## 0.1.1 (2026-11-22)

编辑行为修复（6 类，与 Python tomlkit 实测对齐）：

- 追加/删除定位改为全树行流边界感知：新行不再落进无关表区块（T3/N1 场景），行位不足时平移腾位，`ensureSeqGap` 兜底连续追加
- 构造表补 `owner` 链，上溯定位生效；根表挂 `doc` 反指针
- 构造纯容器表不渲染裸表头（super table 形态，Python 实测裁决）
- 删块 trivia 归位：块前注释保留、块尾空行随删；替换路径不触发转移
- 尾空行区转移矩阵：整表 append / 子表挂载 / 纯 kv 文档三路径
- 新键落表尾注释之后；无尾换行时追加只补末行换行；中位落点复用后继空行区

## 0.1.0 (2026-11-22)

首个公开版本。

- TOML 1.0.0 全语法解析（含多行字符串、内联表、dotted keys、AoT、四种 datetime）
- 保格式文档树：注释、空行、缩进、键序、引号风格全部保留
- 编辑 API 双层：树 API（Get/Set/Delete/Keys）+ 点路径糖（GetPath/SetPath/DeletePath 与 Keys 变体）
- 原位替换保留缩进与行尾注释；整表替换表头原位；新表末尾追加
- 数组元素编辑（Set/Get/Append/Len）保持容器形态；AoT 元素级 Set/Append
- toml-test 官方套件全过（185 valid + 371 invalid，0 skip）
- round-trip identity：toml-test 全集 + 真实样本逐字节一致
- 编辑行为与 Python tomlkit 对拍：11 场景黄金输出逐字节一致
- 已知限制见 README「Known limits (v1)」
