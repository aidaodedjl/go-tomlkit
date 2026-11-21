package tomlkit

// 消费方集成场景验收：deploy_toolbox config.go、TestMotion tomlrw.go
// 行为清单、TestMotion toml_handler.py 三行操作。

import (
	"strings"
	"testing"
)

const deployShape = `# 机器人配置
host = "192.168.1.100"  # 主机
volume = 0.8

[actions]
# 内置动作

[actions.wave]
name = "wave"
tags = ["punchy", "soft"]
unitree_fsm_id = "wave_v2"

[actions.jump]
name = "jump"

[deploy]
targets = ["a", "b"]
`

// deploy_toolbox config.go：改标量保缩进与行尾注释、换整个 actions 表、
// 新键按序追加
func TestConsumerDeployToolbox(t *testing.T) {
	doc := mustParseOk(t, deployShape)

	// 改标量：保留行尾注释与缩进
	if err := doc.Set("host", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Set("volume", 1.0); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	if !strings.Contains(out, `host = "10.0.0.1"  # 主机`) {
		t.Fatalf("标量替换丢注释：%q", out)
	}
	if !strings.Contains(out, "volume = 1.0\n") {
		t.Fatalf("标量替换不符：%q", out)
	}

	// 换整个 actions 表：表头原位，未知扩展字段（unitree_fsm_id）随旧区块
	// 消失（调用方负责全量透传，与 config.py 语义一致）
	nt := NewTable()
	wave := NewTable()
	_ = wave.Set("name", "wave")
	_ = wave.Set("tags", []any{"x"})
	_ = nt.Set("wave", wave)
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	out = doc.String()
	if !strings.Contains(out, "[actions]\n\n[actions.wave]\nname = \"wave\"\ntags = [\"x\"]") {
		t.Fatalf("actions 整表替换不符：%q", out)
	}
	// deploy 区块不受影响
	if !strings.Contains(out, "[deploy]\ntargets = [\"a\", \"b\"]") {
		t.Fatalf("deploy 区块被动：%q", out)
	}

	// 新键按序追加（actions 保序）
	tb, _ := doc.Get("deploy").Table()
	if err := tb.Set("mode", "fast"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), "targets = [\"a\", \"b\"]\nmode = \"fast\"") {
		t.Fatalf("新键追加不符：%q", doc.String())
	}
}

// TestMotion tomlrw.go 行为清单：actions 保文件序、原位换值、表尾插键、
// 整表追加、区间删除
func TestConsumerTestMotionGo(t *testing.T) {
	doc := mustParseOk(t, deployShape)

	// actions 保文件序（tomlrw 的核心约定）
	at, _ := doc.Get("actions").Table()
	if k := at.Keys(); k[0] != "wave" || k[1] != "jump" {
		t.Fatalf("actions 保序失败：%v", k)
	}

	// 原位换值
	if err := doc.SetPath("actions.wave.volume2", 0.5); err != nil {
		t.Fatal(err)
	}
	tb, _ := doc.Get("actions").Table()
	wave, _ := tb.Get("wave").Table()
	if err := wave.Set("name", "wave2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), "name = \"wave2\"") {
		t.Fatal("原位换值失败")
	}

	// 表尾插键
	if err := doc.SetPath("actions.jump.enabled", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.String(), "name = \"jump\"\nenabled = true") {
		t.Fatalf("表尾插键失败：%q", doc.String())
	}

	// 整表追加
	extra := NewTable()
	_ = extra.Set("k", int64(1))
	if err := doc.Set("extra", extra); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(doc.String(), "\n\n[extra]\nk = 1\n") {
		t.Fatalf("整表追加失败：%q", doc.String())
	}

	// 区间删除（删整个 jump 区块）
	if !doc.DeletePath("actions.jump") {
		t.Fatal("区间删除失败")
	}
	if strings.Contains(doc.String(), "[actions.jump]") {
		t.Fatalf("区块残留：%q", doc.String())
	}
}

// TestMotion toml_handler.py：parse → del doc["actions"] + 重建 → dumps
// play_audio 强制 true
func TestConsumerTestMotionPython(t *testing.T) {
	doc := mustParseOk(t, deployShape)
	if err := doc.Set("play_audio", true); err != nil {
		t.Fatal(err)
	}
	nt := NewTable()
	wave := NewTable()
	_ = wave.Set("name", "wave")
	_ = nt.Set("wave", wave)
	if err := doc.Set("actions", nt); err != nil {
		t.Fatal(err)
	}
	out := doc.String()
	// 重解析语义校验（与 tomlkit.dumps 后再 parse 的用法一致）
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Fatal(perr)
	}
	if v, ok := re.Get("play_audio").AsBool(); !ok || !v {
		t.Fatal("play_audio")
	}
	tb, _ := re.Get("actions").Table()
	if len(tb.Keys()) != 1 || tb.Keys()[0] != "wave" {
		t.Fatalf("actions 语义：%v", tb.Keys())
	}
}
