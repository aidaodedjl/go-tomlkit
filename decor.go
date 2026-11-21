package tomlkit

// decor 是条目与表头的格式信息。写回时未修改条目按原文区间重组，
// decor 的使命是让重组结果与输入逐字节一致。
type decor struct {
	prefix string  // 上一条目结束到键名（或表头 [）之间的一切原文：空行、注释、缩进
	keyRaw string  // 键（或表头路径）的原文书写，含引号与点
	sep    string  // kv 分隔的原文，如 "="、" = "、"  =  "
	tail   string  // 值结束到行尾的原文：空白与行尾注释
	eol    string  // "\n"、"\r\n" 或 ""（文件末行无换行）
	dotted bool    // 该条目源自点写法 a.b = 1
	seq    float64 // 文档行序（1-based 行号；追加可取中点），渲染按此排序还原表头/kv 的时序
}
