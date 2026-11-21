# Changelog

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
