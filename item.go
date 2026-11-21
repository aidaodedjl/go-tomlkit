package tomlkit

// ItemType 标识 Item 的具体类型。
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

// Item 是文档树中任意值的接口。类型不匹配的访问器返回零值与 false。
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

// ---- Item 接口的八类型实现（机械分派：自类型命中，其余零值 false）----

func (t *Table) Type() ItemType                    { return ItemTable }
func (t *Table) AsBool() (bool, bool)              { return false, false }
func (t *Table) AsInteger() (int64, bool)          { return 0, false }
func (t *Table) AsFloat() (float64, bool)          { return 0, false }
func (t *Table) AsString() (string, bool)          { return "", false }
func (t *Table) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (t *Table) Table() (*Table, bool)             { return t, true }
func (t *Table) InlineTable() (*InlineTable, bool) { return nil, false }
func (t *Table) Array() (*Array, bool)             { return nil, false }
func (t *Table) Any() any {
	m := make(map[string]any, len(t.items))
	for k, v := range t.items {
		m[k] = v.Any()
	}
	return m
}

func (t *InlineTable) Type() ItemType                    { return ItemInlineTable }
func (t *InlineTable) AsBool() (bool, bool)              { return false, false }
func (t *InlineTable) AsInteger() (int64, bool)          { return 0, false }
func (t *InlineTable) AsFloat() (float64, bool)          { return 0, false }
func (t *InlineTable) AsString() (string, bool)          { return "", false }
func (t *InlineTable) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (t *InlineTable) Table() (*Table, bool)             { return nil, false }
func (t *InlineTable) InlineTable() (*InlineTable, bool) { return t, true }
func (t *InlineTable) Array() (*Array, bool)             { return nil, false }
func (t *InlineTable) Any() any {
	m := make(map[string]any, len(t.items))
	for k, v := range t.items {
		m[k] = v.Any()
	}
	return m
}

func (a *Array) Type() ItemType                    { return ItemArray }
func (a *Array) AsBool() (bool, bool)              { return false, false }
func (a *Array) AsInteger() (int64, bool)          { return 0, false }
func (a *Array) AsFloat() (float64, bool)          { return 0, false }
func (a *Array) AsString() (string, bool)          { return "", false }
func (a *Array) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (a *Array) Table() (*Table, bool)             { return nil, false }
func (a *Array) InlineTable() (*InlineTable, bool) { return nil, false }
func (a *Array) Array() (*Array, bool)             { return a, true }
func (a *Array) Any() any {
	s := make([]any, len(a.items))
	for i, v := range a.items {
		s[i] = v.Any()
	}
	return s
}

func (s *String) Type() ItemType                    { return ItemString }
func (s *String) AsBool() (bool, bool)              { return false, false }
func (s *String) AsInteger() (int64, bool)          { return 0, false }
func (s *String) AsFloat() (float64, bool)          { return 0, false }
func (s *String) AsString() (string, bool)          { return s.v, true }
func (s *String) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (s *String) Table() (*Table, bool)             { return nil, false }
func (s *String) InlineTable() (*InlineTable, bool) { return nil, false }
func (s *String) Array() (*Array, bool)             { return nil, false }
func (s *String) Any() any                          { return s.v }

func (n *Integer) Type() ItemType                    { return ItemInteger }
func (n *Integer) AsBool() (bool, bool)              { return false, false }
func (n *Integer) AsInteger() (int64, bool)          { return n.v, true }
func (n *Integer) AsFloat() (float64, bool)          { return 0, false }
func (n *Integer) AsString() (string, bool)          { return "", false }
func (n *Integer) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (n *Integer) Table() (*Table, bool)             { return nil, false }
func (n *Integer) InlineTable() (*InlineTable, bool) { return nil, false }
func (n *Integer) Array() (*Array, bool)             { return nil, false }
func (n *Integer) Any() any                          { return n.v }

func (f *Float) Type() ItemType                    { return ItemFloat }
func (f *Float) AsBool() (bool, bool)              { return false, false }
func (f *Float) AsInteger() (int64, bool)          { return 0, false }
func (f *Float) AsFloat() (float64, bool)          { return f.v, true }
func (f *Float) AsString() (string, bool)          { return "", false }
func (f *Float) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (f *Float) Table() (*Table, bool)             { return nil, false }
func (f *Float) InlineTable() (*InlineTable, bool) { return nil, false }
func (f *Float) Array() (*Array, bool)             { return nil, false }
func (f *Float) Any() any                          { return f.v }

func (b *Boolean) Type() ItemType                    { return ItemBoolean }
func (b *Boolean) AsBool() (bool, bool)              { return b.v, true }
func (b *Boolean) AsInteger() (int64, bool)          { return 0, false }
func (b *Boolean) AsFloat() (float64, bool)          { return 0, false }
func (b *Boolean) AsString() (string, bool)          { return "", false }
func (b *Boolean) AsDatetime() (Datetime, bool)      { return Datetime{}, false }
func (b *Boolean) Table() (*Table, bool)             { return nil, false }
func (b *Boolean) InlineTable() (*InlineTable, bool) { return nil, false }
func (b *Boolean) Array() (*Array, bool)             { return nil, false }
func (b *Boolean) Any() any                          { return b.v }

func (dt *Datetime) Type() ItemType           { return ItemDatetime }
func (dt *Datetime) AsBool() (bool, bool)     { return false, false }
func (dt *Datetime) AsInteger() (int64, bool) { return 0, false }
func (dt *Datetime) AsFloat() (float64, bool) { return 0, false }
func (dt *Datetime) AsString() (string, bool) { return "", false }
func (dt *Datetime) AsDatetime() (Datetime, bool) {
	return Datetime{Kind: dt.Kind, Time: dt.Time}, true
}
func (dt *Datetime) Table() (*Table, bool)             { return nil, false }
func (dt *Datetime) InlineTable() (*InlineTable, bool) { return nil, false }
func (dt *Datetime) Array() (*Array, bool)             { return nil, false }
func (dt *Datetime) Any() any                          { return Datetime{Kind: dt.Kind, Time: dt.Time} }
