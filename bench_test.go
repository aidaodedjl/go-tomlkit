package tomlkit

import (
	"strings"
	"testing"
)

// 基准：解析、写回、编辑往返（防性能回归的基线，配置文件规模）。

func benchDoc(b *testing.B, n int) string {
	var sb strings.Builder
	sb.WriteString("title = \"benchmark\"\n\n[owner]\nname = \"go\"\n")
	for i := 0; i < n; i++ {
		sb.WriteString("\n[servers.s")
		sb.WriteString(itoa(i))
		sb.WriteString("]\nip = \"10.0.0.")
		sb.WriteString(itoa(i%250 + 1))
		sb.WriteString("\"\nports = [8000, 8001]\n")
	}
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func BenchmarkParse100(b *testing.B)  { benchParse(b, 100) }
func BenchmarkParse1000(b *testing.B) { benchParse(b, 1000) }

func benchParse(b *testing.B, n int) {
	src := benchDoc(b, n)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse([]byte(src)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkString1000(b *testing.B) {
	doc := MustParse([]byte(benchDoc(b, 1000)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = doc.String()
	}
}

func BenchmarkEdit1000(b *testing.B) {
	src := []byte(benchDoc(b, 1000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := MustParse(src)
		_ = doc.SetPath("servers.s500.ip", "1.2.3.4")
		_ = doc.String()
	}
}
