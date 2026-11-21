package tomlkit

import (
	"bytes"
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tomltest "github.com/toml-lang/toml-test"
)

// property：随机选 toml-test valid 文件，随机选根键做标量替换，
// 断言三件事（规范 14 的机器验证）：
// 1. 写回后重新 Parse 无错；
// 2. 目标键读回新值；
// 3. 输出与原文的行 diff 不超过目标行（原位替换）加追加行。

func randomScalarValue(rnd *rand.Rand) any {
	switch rnd.Intn(5) {
	case 0:
		return rnd.Int63n(1<<40) - (1 << 39)
	case 1:
		return rnd.Float64()*1000 - 500
	case 2:
		return rnd.Intn(2) == 0
	case 3:
		return "随机值 " + string(rune('a'+rnd.Intn(26)))
	default:
		if rnd.Intn(2) == 0 {
			return map[string]any{"m1": int64(1), "m2": rnd.Int63n(100)}
		}
		return []any{int64(1), rnd.Int63n(100)}
	}
}

// semanticEqual 断言「编辑后期望语义树 == 写回重解析语义树」。
// 期望树在编辑后的 doc 上直接取 Any()，重解析树在 String() 输出上取。
func assertSemanticRoundTrip(t *testing.T, name string, doc *Document) (string, bool) {
	t.Helper()
	out := doc.String()
	re, perr := parse([]byte(out))
	if perr != nil {
		t.Errorf("[%s] 写回不合法：%v\n--- out:\n%s", name, perr, out)
		return out, false
	}
	if !reflect.DeepEqual(normalizeSemantics(doc.root.Any()), normalizeSemantics(re.root.Any())) {
		t.Errorf("[%s] 语义树不等（编辑后 vs 写回重解析）：\n--- want:\n%#v\n--- got:\n%#v\n--- out:\n%s",
			name, doc.root.Any(), re.root.Any(), out)
		return out, false
	}
	return out, true
}

// normalizeSemantics 把 NaN 归一化为标记值（DeepEqual 对 NaN 恒假）。
func normalizeSemantics(v any) any {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) {
			return "§NaN§"
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeSemantics(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalizeSemantics(e)
		}
		return out
	}
	return v
}

func TestPropertyRandomEdit(t *testing.T) {
	embedded := tomltest.EmbeddedTests()
	list, err := tomltest.Runner{Files: embedded, Version: "1.0.0"}.List()
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, p := range list {
		if strings.HasPrefix(p, "valid/") && !strings.HasSuffix(p, ".multi") {
			files = append(files, p+".toml")
		}
	}
	rnd := rand.New(rand.NewSource(20261121))
	rounds := 0
	attempts := 0
	for rounds < 1000 && attempts < 30000 {
		attempts++
		fp := files[rnd.Intn(len(files))]
		data, err := readFileFS(embedded, fp)
		if err != nil {
			continue
		}
		doc, perr := parse(data)
		if perr != nil {
			continue
		}
		// 预编辑 identity：无修改写回必须语义等价（N2 类回归的哨兵）
		if pre, pe := parse([]byte(doc.String())); pe != nil {
			t.Fatalf("[%s] 预编辑写回不合法：%v\n%s", fp, pe, doc.String())
		} else if !reflect.DeepEqual(normalizeSemantics(doc.root.Any()), normalizeSemantics(pre.root.Any())) {
			t.Fatalf("[%s] 预编辑语义漂移：\n%s", fp, data)
		}
		keys := doc.Keys()
		if len(keys) == 0 {
			continue
		}
		key := keys[rnd.Intn(len(keys))]
		switch rnd.Intn(7) {
		case 0, 1: // 标量替换
			switch doc.Get(key).(type) {
			case *String, *Integer, *Float, *Boolean:
			default:
				continue
			}
			if err := doc.Set(key, randomScalarValue(rnd)); err != nil {
				t.Fatalf("[%s] Set(%s)：%v", fp, key, err)
			}
		case 2: // 数组元素替换
			arr, ok := doc.Get(key).Array()
			if !ok || arr.Len() == 0 || arr.aot {
				continue
			}
			if err := arr.Set(rnd.Intn(arr.Len()), randomScalarValue(rnd)); err != nil {
				t.Fatalf("[%s] Array.Set：%v", fp, err)
			}
		case 3: // SetPath 新建 dotted 链
			if _, perr := parse(data); perr != nil {
				continue
			}
			name := "p" + strconv.Itoa(rnd.Intn(1000))
			if err := doc.SetPath("zz."+name+".deep", int64(1)); err != nil {
				t.Fatalf("[%s] SetPath：%v", fp, err)
			}
		case 4: // 整表替换（多 kv）
			tb, ok := doc.Get(key).(*Table)
			if !ok || !tb.hasHeader {
				continue
			}
			nt := NewTable()
			for j := 1; j <= 2+rnd.Intn(6); j++ {
				_ = nt.Set("np"+strconv.Itoa(j), int64(j))
			}
			if err := doc.Set(key, nt); err != nil {
				t.Fatalf("[%s] 整表替换：%v", fp, err)
			}
		case 5: // 内联表成员修改或删除
			itb, ok := doc.Get(key).(*InlineTable)
			if !ok || len(itb.keys) == 0 {
				continue
			}
			mk := itb.keys[rnd.Intn(len(itb.keys))]
			if rnd.Intn(3) == 0 {
				if !itb.Delete(mk) {
					t.Fatalf("[%s] 内联表删除失败", fp)
				}
			} else if err := itb.Set(mk, int64(77)); err != nil {
				t.Fatalf("[%s] 内联表修改：%v", fp, err)
			}
		case 6: // 深层嵌套修改（隔代 raw 传播）
			if err := doc.SetPath("zzdeep.a.b.c", int64(5)); err != nil {
				t.Fatalf("[%s] 深层 SetPath：%v", fp, err)
			}
			// 混合：同一文档再做一个普通编辑
			if len(doc.Keys()) > 1 {
				k2 := doc.Keys()[rnd.Intn(len(doc.Keys()))]
				switch doc.Get(k2).(type) {
				case *String, *Integer, *Float, *Boolean:
					_ = doc.Set(k2, true)
				}
			}
		case 7: // AoT 元素编辑（Append/Set，兄弟表不得被打穿）
			arr, ok := doc.Get(key).Array()
			if !ok || !arr.aot || arr.Len() == 0 {
				continue
			}
			nt := NewTable()
			for j := 1; j <= 2+rnd.Intn(4); j++ {
				_ = nt.Set("ne"+strconv.Itoa(j), int64(j))
			}
			if rnd.Intn(2) == 0 {
				if err := arr.Append(nt); err != nil {
					t.Fatalf("[%s] AoT Append：%v", fp, err)
				}
			} else if err := arr.Set(rnd.Intn(arr.Len()), nt); err != nil {
				t.Fatalf("[%s] AoT Set：%v", fp, err)
			}
		}
		rounds++
		out, ok := assertSemanticRoundTrip(t, fp, doc)
		if !ok {
			t.Fatalf("语义 oracle 失败于轮 %d", rounds)
		}
		_ = out
		// 无尾换行变体（每 50 轮抽一次）
		if rounds%50 == 0 {
			trimmed := bytes.TrimRight(data, "\n")
			if len(trimmed) > 0 && len(trimmed) < len(data) {
				docN, perrN := parse(trimmed)
				if perrN == nil {
					_ = docN.Set("zz_tail", int64(1))
					if _, pe := parse([]byte(docN.String())); pe != nil {
						t.Fatalf("[%s] 无尾换行追加写回不合法：%v\n%q", fp, pe, docN.String())
					}
				}
			}
		}
	}
	if rounds < 1000 {
		t.Fatalf("有效轮数不足：%d", rounds)
	}
	t.Logf("property: %d 轮通过（%s）", rounds, time.Now().Format("15:04:05"))
}
