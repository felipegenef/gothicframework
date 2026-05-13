# WASM Hooks Reference

All hooks are available via the dot import in any `PageState` function:

```go
import . "github.com/felipegenef/gothicframework/pkg/wasm"
```

They compile as no-ops server-side and as the real reactive TinyGo implementation in the WASM binary.

---

## State

### `UseState[T any](initial T) *Signal[T]`

Creates a reactive state container. When the value changes, every `UseEffect` subscribed to this signal re-runs.

```go
count := UseState(0)
name  := UseState("")
on    := UseState(false)
```

**Signal methods:**

| Method | Description |
|--------|-------------|
| `Get() T` | Returns the current value. Inside a `UseEffect`, this read auto-registers the effect as a subscriber when using auto-tracking; in explicit-deps mode it is just a plain read. |
| `Set(v T)` | Updates the value and re-runs all subscribed effects. |

---

## Effects

### `UseEffect(fn func(), deps ...any) *Effect`

Runs `fn` and re-runs it whenever any listed dep changes.

- **No deps** — runs `fn` exactly once when state loads. No reactive subscription.
- **With deps** — re-runs `fn` whenever any dep's `.Set()` is called.

Deps must be `*Signal[T]` values returned by `UseState`. Anything else is silently skipped in production and prints a `console.warn` in dev mode.

```go
// Run once — fetch initial data, start a timer, etc.
UseEffect(func() {
    SetText("status", "ready")
})

// Re-run whenever `count` changes
UseEffect(func() {
    SetText("counter-display", strconv.Itoa(count.Get()))
}, count)

// Re-run whenever either dep changes
UseEffect(func() {
    if liked.Get() {
        SetText("label", "♥ "+strconv.Itoa(likes.Get()))
    } else {
        SetText("label", "♡ "+strconv.Itoa(likes.Get()))
    }
}, likes, liked)
```

**Returned `*Effect` methods:**

| Method | Description |
|--------|-------------|
| `Stop()` | Unsubscribes the effect from all its deps and deactivates it. |

---

### `UseEffectWithCleanup(fn func() func(), deps ...any) *Effect`

Like `UseEffect`, but `fn` returns a cleanup function. The cleanup runs before each re-execution and when `Stop()` is called.

- **No deps** — runs `fn` once; cleanup return value is discarded.
- **With deps** — cleanup from the previous run is called before the next run.

Useful for timers, subscriptions, or any resource that must be released before re-creating it.

```go
UseEffectWithCleanup(func() func() {
    id := js.Global().Call("setInterval", js.FuncOf(func(js.Value, []js.Value) any {
        tick.Set(tick.Get() + 1)
        return nil
    }), 1000)

    return func() {
        js.Global().Call("clearInterval", id)
    }
}, tick)
```

---

## Batching

### `Batch(fn func())`

Defers all effect re-executions until `fn` returns. When multiple signals are updated together, effects subscribed to more than one of them run only once instead of once per `.Set()` call.

```go
Register("reset", func() {
    Batch(func() {
        count.Set(0)
        label.Set("reset")
        active.Set(false)
    })
    // effects run here, once, after all three sets
})
```

---

## Event Registration

These functions expose Go callbacks to JavaScript. Gothic routes the call to the correct WASM module when multiple components with the same function name are on the same page (see `WASM_SCOPING.md`).

### `Register(name string, fn func())`

Registers a zero-argument callback callable from HTML event attributes.

```go
Register("increment", func() {
    count.Set(count.Get() + 1)
})
```

```html
<button onclick="increment()">+</button>
```

---

### `RegisterInput(name string, fn func(string))`

Registers a callback that receives a string value. Use this for text input handlers where the value is passed as an argument.

```go
RegisterInput("setName", func(val string) {
    name.Set(val)
})
```

```html
<input oninput="setName(this.value)" />
```

---

### `RegisterBool(name string, fn func(bool))`

Registers a callback that receives a boolean value. Use this for checkboxes.

```go
RegisterBool("setChecked", func(val bool) {
    checked.Set(val)
})
```

```html
<input type="checkbox" onchange="setChecked(this.checked)" />
```

---

## DOM Helpers

All helpers operate by element ID. They are safe to call with an unknown ID — missing elements are silently skipped.

### Text and HTML

| Function | Description |
|----------|-------------|
| `SetText(id, value string)` | Sets `element.textContent`. Escapes HTML — use for plain text. |
| `SetHTML(id, html string)` | Sets `element.innerHTML`. Use only with trusted content. |

```go
SetText("message", "Hello, world!")
SetHTML("card-body", "<strong>bold</strong> content")
```

---

### Form Values

| Function | Description |
|----------|-------------|
| `SetValue(id, value string)` | Sets `element.value` — for `<input>`, `<textarea>`, `<select>`. |
| `GetValue(id string) string` | Returns `element.value`. Returns `""` if element not found. |

```go
RegisterInput("submit", func(val string) {
    last := GetValue("my-input")
    SetValue("my-input", "")
    _ = last
})
```

---

### CSS Classes

| Function | Description |
|----------|-------------|
| `AddClass(id, className string)` | Adds a CSS class to the element. |
| `RemoveClass(id, className string)` | Removes a CSS class from the element. |
| `ToggleClass(id, className string)` | Toggles a CSS class on the element. |

```go
UseEffect(func() {
    if open.Get() {
        RemoveClass("menu", "hidden")
        AddClass("overlay", "opacity-50")
    } else {
        AddClass("menu", "hidden")
        RemoveClass("overlay", "opacity-50")
    }
}, open)
```

---

### Attributes and Styles

| Function | Description |
|----------|-------------|
| `SetAttr(id, attr, value string)` | Calls `element.setAttribute(attr, value)`. |
| `SetStyle(id, property, value string)` | Sets `element.style[property] = value`. Property names are camelCase JS names (e.g. `"backgroundColor"`, not `"background-color"`). |

```go
// Toggle aria state
SetAttr("dialog", "aria-hidden", "false")

// Dynamic inline style
SetStyle("progress-bar", "width", strconv.Itoa(pct.Get())+"%")
SetStyle("preview-swatch", "backgroundColor", hex.Get())
```

---

## Context

Context lets multiple WASM components share reactive state without prop drilling. Because each component is a separate WASM module with its own Go heap, values are serialized through a JavaScript store (`window.__gothic_context`) and broadcast via `CustomEvent`. The API mirrors the signal model — consumers get a regular `*Signal[T]` that works with `UseEffect` like any other signal.

### Key factories

Each factory returns a `ContextKey[T]` that carries its own codec — no encode/decode ever appears at the call site.

**Primitives** — lightweight, no extra binary cost:

| Factory | Type |
|---------|------|
| `BoolKey(name)` | `ContextKey[bool]` |
| `StringKey(name)` | `ContextKey[string]` |
| `IntKey(name)` | `ContextKey[int]` |
| `Int8Key(name)` | `ContextKey[int8]` |
| `Int16Key(name)` | `ContextKey[int16]` |
| `Int32Key(name)` | `ContextKey[int32]` |
| `Int64Key(name)` | `ContextKey[int64]` |
| `UintKey(name)` | `ContextKey[uint]` |
| `Uint8Key(name)` | `ContextKey[uint8]` |
| `Uint16Key(name)` | `ContextKey[uint16]` |
| `Uint32Key(name)` | `ContextKey[uint32]` |
| `Uint64Key(name)` | `ContextKey[uint64]` |
| `Float32Key(name)` | `ContextKey[float32]` |
| `Float64Key(name)` | `ContextKey[float64]` |
| `RuneKey(name)` | `ContextKey[rune]` (= int32) |
| `ByteKey(name)` | `ContextKey[byte]` (= uint8) |

**JSON** — structs, slices, maps — serialized as JSON:

| Factory | Type |
|---------|------|
| `JsonKey[T any](name)` | `ContextKey[T]` |

`T` must be JSON-serializable (exported fields, no channels or functions). `JsonKey` pulls in `encoding/json`, which adds ~140 KB gzip to any binary that calls it. Binaries using only primitive keys are unaffected.

> **Future consideration — drop `encoding/json` from WASM binaries**
>
> The size cost comes from TinyGo having to compile Go's reflection engine to support generic marshal/unmarshal. A planned improvement would use `syscall/js` on the WASM side to delegate serialization to the browser's native `JSON.stringify`/`JSON.parse` (already present, zero extra binary cost), while keeping `encoding/json` only in the server-side stub (which does not affect WASM binary size). The missing piece is the Go struct ↔ `js.Value` bridge: `js.ValueOf` accepts `map[string]interface{}` but not arbitrary structs, so the conversion still needs either reflection or a small interface the user implements once per type (`GothicEncode() map[string]any` / `GothicDecode(js.Value)`). If that interface approach is acceptable, `JsonKey` binaries could drop back to the same ~21 KB as primitive-key binaries.

---

### `ProvideContext[T any](key ContextKey[T], signal *Signal[T])`

Makes `signal` the source of truth for the named context. Uses auto-tracking internally — whenever `signal` changes the encoded value is written to `window.__gothic_context[key.Name]` and broadcast via `CustomEvent` so all consumers update reactively.

```go
// Primitive — single int
pings := UseState(0)
ProvideContext(IntKey("page-pings"), pings)

// Struct — multiple fields bundled together
type PageCtx struct { Pings int; Label string }
ctx := UseState(PageCtx{Pings: 0, Label: "ready"})
ProvideContext(JsonKey[PageCtx]("page-ctx"), ctx)
```

---

### `UseContext[T any](key ContextKey[T], initial T) *Signal[T]`

Subscribes to the named context. Reads the current value from `window.__gothic_context` at startup (handles components that load after the provider already ran), then listens for future updates. Returns a local `*Signal[T]` — use as a dep in `UseEffect` like any other signal.

```go
// Primitive
pings := UseContext(IntKey("page-pings"), 0)
UseEffect(func() {
    SetText("pm-count", strconv.Itoa(pings.Get()))
}, pings)

// Struct — PageCtx type must be visible in this module (see "Shared types" below)
pageCtx := UseContext(JsonKey[PageCtx]("page-ctx"), PageCtx{})
UseEffect(func() {
    SetText("pm-label", pageCtx.Get().Label)
}, pageCtx)
```

---

### Shared types — declaring structs once (Path A and Path B)

For primitive contexts (`IntKey`, `StringKey`, etc.) there is nothing to share — just use the same factory call in provider and consumer.

For struct contexts (`JsonKey[T]`) the struct type must be visible in both the provider and consumer modules. There are two ways to achieve this:

#### Path A — inline structs (no extra setup)

Define the same struct inside each `PageState` body that uses it. Go's `encoding/json` is shape-based — as long as field names and `json:` tags match, round-trips work regardless of Go type identity.

```go
// In page PageState — provider:
type PageCtx struct{ Pings int `json:"pings"`; Label string `json:"label"` }
ctx := UseState(PageCtx{})
ProvideContext(JsonKey[PageCtx]("page-ctx"), ctx)

// In component PageState — consumer (identical struct definition):
type PageCtx struct{ Pings int `json:"pings"`; Label string `json:"label"` }
pageCtx := UseContext(JsonKey[PageCtx]("page-ctx"), PageCtx{})
```

Best for: small structs that are only used by one or two modules.

#### Path B — shared `src/wasm/` directory (single source of truth)

Create `src/wasm/contexts.go` with `package gothicwasm`. The CLI automatically bundles every `.go` file in this directory into every WASM binary. The package declaration is rewritten to `package main` at compile time, so all exported types are directly available in `PageState` bodies without any import.

```
src/
  wasm/
    contexts.go   ← type definitions, package gothicwasm, no imports
```

```go
// src/wasm/contexts.go
package gothicwasm

// Rules:
//   - package must be named gothicwasm
//   - no imports — pure type definitions only
//   - all fields exported, JSON-serializable

type PageCtx struct {
    Pings int    `json:"pings"`
    Label string `json:"label"`
}
```

For server-side compilation (Go, not TinyGo), templ files import the same package normally:

```go
import . "yourmodule/src/wasm"  // dot import — PageCtx available without prefix
```

Then in any `PageState` body (WASM or server):
```go
// Works in both — no import line needed in PageState body itself
ctx := UseState(PageCtx{Pings: 0, Label: "ready"})
ProvideContext(JsonKey[PageCtx]("page-ctx"), ctx)
```

Best for: structs used across many components, or when you want a single canonical definition.

**Rules for `src/wasm/` files:**
- Package name must be `gothicwasm`
- No imports of any kind — only plain Go type definitions
- All struct fields must be exported and JSON-serializable
- Build tags are not needed — the CLI includes these files only during WASM compilation

---

### How it works

Each WASM module runs in its own Go heap — `*Signal[T]` pointers cannot cross module boundaries. `ProvideContext` writes the serialized value to `window.__gothic_context[name]` and fires a `CustomEvent("gothic:context:name")` on `document`. `UseContext` reads the store at startup for the initial value, then listens for that event and calls `.Set()` on the local signal. From the consumer's perspective it is an ordinary signal — subscribe to it in `UseEffect`, read it with `.Get()`.

---

## Complete example

```go
PageState: func() {
    count := UseState(0)
    step  := UseState(1)

    UseEffect(func() {
        SetText("count-display", strconv.Itoa(count.Get()))
        SetText("total-display", strconv.Itoa(count.Get()*step.Get()))
    }, count, step)

    Register("increment", func() { count.Set(count.Get() + step.Get()) })
    Register("decrement", func() { count.Set(count.Get() - step.Get()) })
    Register("reset", func() {
        Batch(func() {
            count.Set(0)
            step.Set(1)
        })
    })

    RegisterInput("setStep", func(val string) {
        if n, err := strconv.Atoi(val); err == nil && n > 0 {
            step.Set(n)
        }
    })

    select {}
},
```
