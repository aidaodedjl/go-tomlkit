# go-tomlkit

Round-trip faithful TOML editing for Go — the missing counterpart to Python's
[tomlkit](https://github.com/sdispater/tomlkit) and Rust's
[toml-edit](https://github.com/toml-rs/toml-edit).

BurntSushi/toml and pelletier/go-toml parse TOML into values and re-serialize
the whole document: comments, blank lines, key order, and quoting styles are
lost. go-tomlkit parses into a document tree that keeps every byte of
formatting, lets you edit values in place, and writes back with **untouched
parts byte-for-byte identical**.

```go
doc, err := tomlkit.Parse(data)
if err != nil {
    return err
}
if err := doc.Set("play_audio", true); err != nil {
    return err
}
if err := os.WriteFile(path, doc.Bytes(), 0o644); err != nil {
    return err
}
```

## Install

```sh
go get github.com/aidaodedjl/go-tomlkit
```

Zero third-party runtime dependencies. TOML 1.0.0.

## Guarantees

- `Parse` then `String` with no edits returns the input **byte for byte**
  (verified against the full [toml-test](https://github.com/toml-lang/toml-test)
  valid corpus, including BOM and CRLF files).
- Replacing a value keeps the key's spelling, indentation, and trailing comment.
- Appending a key lands at the end of its table's key area, never after a
  sub-table header (which would change what the key belongs to).
- Deleting a key removes its line and its leading comments; everything else
  stays identical.
- The output always re-parses to the same semantics as "original file plus
  your edits" (property-tested over 1000 random edit rounds).
- Invalid TOML is rejected with line/column/snippet errors — `ParseError.Detailed()`
  renders the offending line with a caret for end users. Nothing is ever silently
  mangled.
- Go native fuzz target over the identity + re-parse invariants, seeded from
  toml-test by category.

Not goals: reformatting/pretty-printing, TOML 1.1 draft features, struct
unmarshalling (BurntSushi already does that), concurrency safety.
Path sugar does not traverse arrays of tables — navigate `Get("x").Array()`
and edit elements (`Set(i, *Table)`, `Append(*Table)`) instead.
`Set` accepts `bool`, `int`, `int64`, `float64`, `string`, `Datetime`,
`[]any`, `map[string]any`, and `Item` values; other types return an error.
`SetPath` stores `*Table` values as inline tables (dot-path values are
value-shaped by definition).

### Known limits (v1)

- Non-contiguous dotted groups inside one inline table
  (`{b.c = 1, d = 2, b.e = 3}`) keep their semantics on write-back, but
  member order may be normalized.
- Deep `SetPath` combinations (3+ dotted levels interleaved with nested
  table headers inside inline tables) are not fully covered; edits there
  stay semantically correct in the tree but written positions may shift.
- Editing an inline table normalizes separators between untouched members
  to `", "`.
- New paths created under a header-defined implicit table (e.g. adding
  `fruit.x` when only `[[fruit.y]]` exists) are written as a dotted key
  line at the top level; Python tomlkit appends a `[fruit.x]` block at the
  end instead. Semantics are identical; the rendering convention differs.

## Usage

### Read and edit in place

```go
doc := tomlkit.MustParse([]byte("volume = 0.8  # keep me\n"))
if err := doc.Set("volume", 1.0); err != nil {
    return err
}
doc.String() // "volume = 1.0  # keep me\n"
```

### Dot-path sugar

```go
v, ok := doc.GetPath("actions.wave.volume").AsFloat()
err := doc.SetPath("actions.wave.volume", 0.5) // creates tables as needed
ok = doc.DeletePath("actions.wave")
// keys containing dots: the explicit Keys variants
doc.SetPathKeys([]string{"a.b", "c"}, 1)
```

### Typed access

```go
if it := doc.Get("enabled"); it != nil {
    if b, ok := it.AsBool(); ok && b { ... }
}
anyValue := doc.Get("actions").Any() // map[string]any tree, no formatting
```

### Replace a whole table (header stays in place)

```go
actions := tomlkit.NewTable()
wave := tomlkit.NewTable()
_ = wave.Set("name", "wave")
_ = actions.Set("wave", wave)
if err := doc.Set("actions", actions); err != nil { // [actions] keeps its line
    return err
}
```

### Build from scratch

```go
doc := tomlkit.NewDocument()
doc.Set("title", "demo")
tbl := tomlkit.NewTable()
tbl.Set("x", 1)
doc.Set("tbl", tbl)
```

## Performance

Benchmarked on the repo (see `bench_test.go`; Apple Silicon, Go 1.27):

```
BenchmarkParse100       ~9.5 MB/s    (100-table config)
BenchmarkParse1000     ~11.8 MB/s    (1000-table config)
BenchmarkString1000     0.9 ms/op    (write-back, no re-render of untouched)
BenchmarkEdit1000       3.6 ms/op    (parse + one SetPath + write-back)
```

This is a fidelity-first library: the tree keeps every byte of formatting,
so parse throughput is not the goal (BurntSushi/go-toml are 5-10x faster
parsers). For config-file editing workloads it is instant.

## Splitting work with BurntSushi/toml

Use BurntSushi when you decode into structs and re-write from scratch.
Use go-tomlkit when the file belongs to a human and must come back the way
they left it.

## License

MIT.
