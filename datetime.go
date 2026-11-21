package tomlkit

import "time"

// DatetimeKind 区分 TOML 的四种日期时间形态。
type DatetimeKind int8

const (
	KindOffsetDatetime DatetimeKind = iota // 1979-05-27T07:32:00Z
	KindLocalDatetime                      // 1979-05-27T07:32:00
	KindLocalDate                          // 1979-05-27
	KindLocalTime                          // 07:32:00
)

// Datetime 是 TOML 日期时间值。local date/time 用零日期承载时钟字段。
type Datetime struct {
	Kind DatetimeKind
	Time time.Time
	d    decor
	raw  string
}
