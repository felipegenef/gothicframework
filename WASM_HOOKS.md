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

### `ContextKey[T any]`

A typed identifier that links a provider to its consumers. `T` encodes the value type — if provider and consumer use different `T` for the same `Name`, the decode function will produce garbage or panic. Declare the key as a literal in each `PageState` that uses it.

```go
ContextKey[int]{"page-pings"}
ContextKey[string]{"theme"}
ContextKey[bool]{"logged-in"}
```

---

### `ProvideContext[T any](key ContextKey[T], signal *Signal[T], encode func(T) string)`

Makes `signal` the source of truth for the named context. Uses auto-tracking internally — whenever `signal` changes, the new value is written to `window.__gothic_context[key.Name]` and broadcast via a `CustomEvent` so all consumers update reactively.

Call this once in the provider's `PageState`, after the signal is created.

```go
pings := UseState(0)
ProvideContext(ContextKey[int]{"page-pings"}, pings, strconv.Itoa)
```

---

### `UseContext[T any](key ContextKey[T], initial T, decode func(string) T) *Signal[T]`

Subscribes to a named context. Reads the current value from `window.__gothic_context` at startup (so it picks up whatever the provider already set), then registers a JS event listener for future updates. Returns a local `*Signal[T]` — use it exactly like any `UseState` signal.

```go
pings := UseContext(ContextKey[int]{"page-pings"}, 0, func(s string) int {
    n, _ := strconv.Atoi(s)
    return n
})
UseEffect(func() {
    SetText("pm-count", strconv.Itoa(pings.Get()))
}, pings)
```

---

### Encode / decode patterns

| Type | encode | decode |
|------|--------|--------|
| `int` | `strconv.Itoa` | `func(s string) int { n, _ := strconv.Atoi(s); return n }` |
| `string` | `func(s string) string { return s }` | `func(s string) string { return s }` |
| `bool` | `strconv.FormatBool` | `func(s string) bool { b, _ := strconv.ParseBool(s); return b }` |
| `float64` | `func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }` | `func(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }` |

---

### Multiple fields

Each field that needs independent reactivity gets its own `ContextKey`. Consumers only subscribe to the keys they care about — a change to `"theme"` won't re-run effects that only depend on `"page-pings"`.

```go
// Provider (page PageState):
pings  := UseState(0)
theme  := UseState("dark")
ProvideContext(ContextKey[int]{"page-pings"}, pings, strconv.Itoa)
ProvideContext(ContextKey[string]{"theme"}, theme, func(s string) string { return s })

// Consumer A — only cares about pings:
pings := UseContext(ContextKey[int]{"page-pings"}, 0, func(s string) int {
    n, _ := strconv.Atoi(s)
    return n
})

// Consumer B — only cares about theme:
theme := UseContext(ContextKey[string]{"theme"}, "dark", func(s string) string { return s })
```

---

### How it works

Each WASM module runs in its own Go heap — `*Signal[T]` pointers cannot cross module boundaries. `ProvideContext` writes serialized values to `window.__gothic_context[name]` and fires a `CustomEvent("gothic:context:name")` on `document`. `UseContext` reads the store at startup for the initial value, then listens for that event to call `.Set()` on the local signal. From the consumer's perspective it is an ordinary signal.

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
