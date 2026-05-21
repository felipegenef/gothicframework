# WASM Runtime — `pkg/helpers/wasm`

This package is the **server-side code-generation engine** for Gothic's WASM feature. It lives inside the CLI (`gothic-cli`) and is never imported by user code directly.

When you run `gothicframework wasm`, this package:
1. Parses your `.templ` files with the Go AST to extract `ClientSideState` functions and their referenced types.
2. Generates a typed codec (encode/decode) for every struct used as WASM context state.
3. Rewrites the WASM entry point source and compiles it with TinyGo (`-gc conservative -target wasm`).

The **user-facing API** (what you actually call inside `ClientSideState`) lives in `pkg/wasm` and is documented below. That package exposes no-op stubs for server-side compilation and the real TinyGo implementations for the WASM binary.

---

# WASM Hooks Reference

All hooks are available via the dot import in any `ClientSideState` function:

```go
import . "github.com/felipegenef/gothicframework/pkg/wasm"
```

They compile as no-ops server-side and as the real reactive TinyGo implementation in the WASM binary.

---

## Helper functions and tree-shaking

Any same-package function, constant, or type referenced (directly or transitively) inside `ClientSideState` is automatically inlined into the generated WASM binary. You do not need to copy helpers manually.

```go
func clamp(v, lo, hi int) int {
    if v < lo { return lo }
    if v > hi { return hi }
    return v
}

var CounterConfig = routes.RouteConfig[CounterProps]{
    ClientSideState: func() {
        count := CreateObservable(0)
        Observe(func() {
            SetText("display", strconv.Itoa(clamp(count.Get(), 0, 100)))
        }, count)
        CreateWasmFunc("inc", func() { count.Set(count.Get() + 1) })
    },
}
```

`clamp` is tree-shaken into the WASM main automatically.

**Rules:**
- Only `func`, `const`, and `type` declarations can be tree-shaken. Package-level `var` references produce a build error with a `file:line:col` position.
- Tree-shaking is recursive — if `clamp` calls another same-package helper, that helper is pulled in too.
- Imports used by pulled helpers are included automatically.
- `init()` functions cannot be referenced and will produce a build error.

---

## State

### `CreateObservable[T any](initial T) *Observable[T]`

Creates a reactive state container. When the value changes, every `Observe` subscribed to this observable re-runs.

```go
count := CreateObservable(0)
name  := CreateObservable("")
on    := CreateObservable(false)
```

**`*Observable[T]` methods:**

| Method | Description |
|--------|-------------|
| `Get() T` | Returns the current value. Always a plain read — `Observe` requires explicit dependency arguments. |
| `Set(v T)` | Updates the value and re-runs all subscribed effects. |

---

## Effects

### `Observe(fn func(), deps ...any) *Subscription`

Runs `fn` and re-runs it whenever any listed dep changes.

- **No deps** — runs `fn` exactly once when state loads. No reactive subscription.
- **With deps** — re-runs `fn` whenever any dep's `.Set()` is called.

Deps must be `*Observable[T]` values returned by `CreateObservable` (or compatible — e.g. `*ObservableField[T]` from generated context structs). Anything else is silently skipped in production and prints a `console.warn` in dev mode.

```go
// Run once — kick off something at startup.
Observe(func() {
    SetText("status", "ready")
})

// Re-run whenever `count` changes
Observe(func() {
    SetText("counter-display", strconv.Itoa(count.Get()))
}, count)

// Re-run whenever either dep changes
Observe(func() {
    if liked.Get() {
        SetText("label", "♥ "+strconv.Itoa(likes.Get()))
    } else {
        SetText("label", "♡ "+strconv.Itoa(likes.Get()))
    }
}, likes, liked)
```

**Returned `*Subscription` methods:**

| Method | Description |
|--------|-------------|
| `Stop()` | Unsubscribes the effect from all its deps and deactivates it. |

---

### `ObserveWithCleanup(fn func() func(), deps ...any) *Subscription`

Like `Observe`, but `fn` returns a cleanup function. The cleanup runs before each re-execution and when `Stop()` is called.

- **No deps** — runs `fn` once; cleanup return value is discarded.
- **With deps** — cleanup from the previous run is called before the next run.

Useful for timers, subscriptions, or any resource that must be released before re-creating it.

```go
ObserveWithCleanup(func() func() {
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

## Event Registration

These functions expose Go callbacks to JavaScript. When multiple WASM modules are on the same page (e.g. a counter component + a menu component + a multiselect), they could all register a function named `"increment"`. Without isolation, each call to `js.Global().Set("increment", f)` overwrites the previous one — the last module to load wins, silently breaking all others.

Gothic solves this transparently with **per-instance scoping** — no changes to user code required.

### How scoping works

Each WASM component gets a unique scope ID generated at render time (e.g. `counter-a3f9b21c`). The bootstrap script stamps this ID as `data-gothic-scope` on the component's root DOM element and sets `window.__gothicCurrentModule` before calling `go.run()`. The WASM module captures that ID in a package-level `cachedModuleID` on init.

`CreateWasmFunc` / `CreateWasmStringFunc` / `CreateWasmBoolFunc` store the callback in `window.__gothic_registry[instanceID][name]` instead of directly on `window`. A thin proxy is created on `window[name]` once per function name. When a user triggers the event, the proxy reads `window.__gothicFindScope()` (which calls `event.target.closest('[data-gothic-scope]')`) to find which instance owns the event and routes to the correct module's callback.

```
User clicks a button inside component A
  │
  ▼
window.increment()       ← proxy, created once for this function name
  │
  ├── __gothicFindScope()
  │     event.target.closest('[data-gothic-scope]')
  │     → "counter-a3f9b21c"
  │
  └── __gothic_registry["counter-a3f9b21c"]["increment"]()
        → module A's callback only ✓
```

**Full-page components** — the scope is stamped as a `data-gothic-scope` attribute on the `<body>` tag.  
**Fragment components** — the content is wrapped in `<div style="display:contents">` (invisible to CSS flexbox/grid) so `closest('[data-gothic-scope]')` can find it.

### Known limitation

The proxy relies on `window.event` being set, which is true for all user-triggered interactions (click, input, change, focus, blur). It is `undefined` for programmatic calls from async contexts (`setTimeout`, Promise callbacks). In those cases the proxy falls back to the first registered module that has the function — which is correct when there is only one instance of a component on the page.

### Why stateful components must lazy-load

Every component with a `ClientSideState` function gets its own `.wasm.gz` / `.wasm.br` file (depending on the `WasmCompression` setting) and its own bootstrap `<script>` tag. The script sets `window.__gothicCurrentModule` immediately before `go.run()` so the module captures the right namespace.

This only works if **each WASM module starts after its scope element is already in the DOM**. If stateful components are inlined in the initial SSR output, all their `<script>` tags fire in parallel. `window.__gothicCurrentModule` gets overwritten by whichever `fetch` resolves last — every module that loaded after the first one captures the wrong namespace.

The fix: load each stateful component as a separate HTMX request after the page is ready. Use `StatefulComponentOf` to do this type-safely:

```go
import gothicComponents "github.com/felipegenef/gothicframework/components"

// Type-safe — path comes from the registered config, no magic strings.
@gothicComponents.StatefulComponentOf(&components.CounterWidgetConfig)

// With a custom loading placeholder:
@gothicComponents.StatefulComponent(components.CounterWidgetConfig.Path) {
    <div class="animate-pulse">Loading…</div>
}
```

The old manual pattern — `<div hx-get="/components/counterwidget" hx-trigger="load" hx-swap="outerHTML">` — still works but has no compile-time path check and breaks silently on rename.

### `CreateWasmFunc(name string, fn func())`

Registers a zero-argument callback callable from HTML event attributes.

```go
CreateWasmFunc("increment", func() {
    count.Set(count.Get() + 1)
})
```

```html
<button onclick="increment()">+</button>
```

---

### `CreateWasmStringFunc(name string, fn func(string))`

Registers a callback that receives a string value. Use this for text input handlers where the value is passed as an argument.

```go
CreateWasmStringFunc("setName", func(val string) {
    name.Set(val)
})
```

```html
<input oninput="setName(this.value)" />
```

---

### `CreateWasmBoolFunc(name string, fn func(bool))`

Registers a callback that receives a boolean value. Use this for checkboxes.

```go
CreateWasmBoolFunc("setChecked", func(val bool) {
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
CreateWasmStringFunc("submit", func(val string) {
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
Observe(func() {
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
SetAttr("dialog", "aria-hidden", "false")
SetStyle("progress-bar", "width", strconv.Itoa(pct.Get())+"%")
SetStyle("preview-swatch", "backgroundColor", hex.Get())
```

---

## HTTP

### `Fetch(url string, config ...FetchConfig) (string, error)`

Makes an HTTP request using the browser's `fetch` API and blocks until complete. Returns the response body as a string, or an error if the request or response reading fails.

Config is optional — omit it for a simple GET.

Must be called from inside a goroutine or `CreateWasmFunc` handler (not at the top level of `ClientSideState`).

```go
// Simple GET
CreateWasmFunc("load", func() {
    body, err := Fetch("https://api.example.com/todos/1")
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    SetText("result", body)
})

// POST with JSON body and headers
CreateWasmFunc("submit", func() {
    body, err := Fetch("https://api.example.com/todos", FetchConfig{
        Method:  "POST",
        Headers: map[string]string{"Content-Type": "application/json"},
        Body:    `{"title":"buy milk","completed":false}`,
    })
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    SetText("result", body)
})

// GET with query parameters
CreateWasmFunc("search", func() {
    body, err := Fetch("https://api.example.com/todos", FetchConfig{
        Query: map[string]string{"userId": "1", "completed": "false"},
    })
    if err != nil {
        fmt.Println("error:", err)
        return
    }
    SetText("result", body)
})
```

**`FetchConfig` fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Method` | `string` | `"GET"` | HTTP method: `"GET"`, `"POST"`, `"PUT"`, `"DELETE"`, etc. |
| `Headers` | `map[string]string` | `nil` | Request headers. |
| `Body` | `string` | `""` | Text request body — use for JSON or form data. |
| `BodyBytes` | `[]byte` | `nil` | Binary request body — used when `Body` is empty. Use for file uploads or raw binary payloads. |
| `Query` | `map[string]string` | `nil` | Query parameters appended to the URL (`?key=value&...`). Values are URL-encoded automatically. |

**Note:** `Fetch` is subject to the browser's CORS policy — cross-origin requests require the server to include appropriate `Access-Control-Allow-Origin` headers.

---

### `FetchBytes(url string, config ...FetchConfig) ([]byte, error)`

Same as `Fetch` but returns the raw response body as `[]byte` instead of a string. Use this when the server returns binary data (images, files, compressed payloads) that would be corrupted by text decoding.

```go
CreateWasmFunc("downloadFile", func() {
    data, err := FetchBytes("/api/downloadTxt")
    if err != nil {
        SetText("result", "error: "+err.Error())
        return
    }
    // data is []byte — convert to string for text, or process as binary
    SetText("result", string(data))
})
```

Accepts the same `FetchConfig` options as `Fetch` (method, headers, body, query params).

**Note:** Internally uses `arrayBuffer()` instead of `text()` on the JS `Response` object, which preserves every byte without any encoding conversion.

---

### `GetFileBytes(id string) []byte`

Reads the contents of the first file selected in a `<input type="file">` element and returns it as `[]byte`. Blocks until the browser's `FileReader` finishes. Returns `nil` if the element is not found, no file is selected, or reading fails.

```go
// HTML
// <input type="file" id="upload" />
// <button onclick="uploadFile()">Upload</button>

CreateWasmFunc("uploadFile", func() {
    data := GetFileBytes("upload")
    if data == nil {
        SetText("status", "no file selected")
        return
    }

    // Send the whole file as a binary body
    _, err := Fetch("https://api.example.com/upload", FetchConfig{
        Method:    "POST",
        Headers:   map[string]string{"Content-Type": "application/octet-stream"},
        BodyBytes: data,
    })
    if err != nil {
        SetText("status", "upload failed: "+err.Error())
        return
    }
    SetText("status", "uploaded!")
})
```

**Chunked upload example** — split the file and send each chunk with a `Content-Range` header:

```go
CreateWasmFunc("uploadChunked", func() {
    data := GetFileBytes("upload")
    if data == nil {
        return
    }

    const chunkSize = 512 * 1024 // 512 KB per chunk
    total := len(data)

    for start := 0; start < total; start += chunkSize {
        end := start + chunkSize
        if end > total {
            end = total
        }
        chunk := data[start:end]

        contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end-1, total)
        _, err := Fetch("https://api.example.com/upload", FetchConfig{
            Method:    "POST",
            Headers:   map[string]string{
                "Content-Type":  "application/octet-stream",
                "Content-Range": contentRange,
            },
            BodyBytes: chunk,
        })
        if err != nil {
            SetText("status", fmt.Sprintf("chunk %d failed: %s", start, err.Error()))
            return
        }
        pct := end * 100 / total
        SetText("status", fmt.Sprintf("uploading... %d%%", pct))
    }
    SetText("status", "done!")
})
```

---

## Context

Context lets multiple WASM components share reactive state without prop drilling. Because each component is a separate WASM module with its own Go heap, values are serialized through a JavaScript store (`window.__gothic_context`) and broadcast via `CustomEvent`. The current API uses a generated context constructor per shared struct — define the struct once in `src/context/` and the CLI generates a `<Struct>Context()` factory that handles encoding, decoding, broadcast, and subscription.

### Defining a shared context

1. Create `src/context/page_context.go` (or any name) with a struct embedding `GothicSharedContext`:

   ```go
   package gothicwasm // any package name works — match what's in your src/context/ files

   import . "github.com/felipegenef/gothicframework/pkg/wasm"

   type Page struct {
       GothicSharedContext `name:"page" compression:"brotli"`
       Pings int
       Label string
       Theme string
   }
   ```

   The `name:` tag on `GothicSharedContext` sets the context key used in JS events (e.g. `gothic:context:page:Theme`). The optional `compression:` tag selects `brotli` or `gzip` for the manager WASM binary (default: `gzip`).

   **Supported field types:**

   | Category | Examples |
   |----------|---------|
   | Primitives | `bool`, `int`, `int8/16/32/64`, `uint`, `uint8/16/32/64`, `float32/64`, `string`, `byte`, `[]byte` |
   | Type aliases | `type MyScore int`, `type Flag bool` — resolved automatically |
   | Slices | `[]string`, `[]Item`, `[]*Item`, `[][]string` |
   | Maps | `map[string]int`, `map[string]Item`, `map[string]*Item`, `map[string]map[string]int` |
   | Pointers | `*string`, `*Item` |
   | Nested structs | `Item` (value), `*Item` (pointer) |
   | Time | `time.Time` |

   Nested maps (`map[K]map[K2]V`) are supported as long as the innermost value is a primitive, type alias, or known struct. Three or more levels of map nesting are not supported and will produce a build-time error.

   The `gothic:` tag on individual fields is an **encoding override** for `int`/`uint` fields where Go's default `int` (platform-width) needs an explicit wire size:

   | Tag | Wire type | Use when |
   |-----|-----------|----------|
   | `gothic:"i32"` | signed 32-bit | `int` field, want 32-bit wire |
   | `gothic:"i64"` | signed 64-bit | `int` field, want 64-bit wire |
   | `gothic:"u32"` | unsigned 32-bit | `uint` field, want 32-bit wire |
   | `gothic:"u64"` | unsigned 64-bit | `uint` field, want 64-bit wire |
   | `gothic:"skip"` | (omitted) | exclude field from wire format |

   Without a `gothic:` tag the CLI infers the codec from the field's Go type.

2. In any `ClientSideState`, call the auto-generated `PageContext()` constructor (the name is `<StructName>Context`):

   ```go
   ClientSideState: func() {
       ctx := PageContext()         // *PageContext
       Observe(func() {
           ctx.Set(Page{Pings: pings.Get(), Label: "...", Theme: theme.Get()})
       }, pings, theme)
   }
   ```

3. Any other module on the page that also calls `PageContext()` receives the same updates. Each field on the returned struct is an `*ObservableField[T]` that participates in `Observe` like a regular observable.

**`*ObservableField[T]` methods:**

| Method | Description |
|--------|-------------|
| `Get() T` | Returns the current value. Pass this field as a dep to `Observe` to react to remote updates. |
| `Peek() T` | Returns the current value without registering a dependency. Safe to call outside `Observe`. |
| `Set(v T)` | Updates the local value and broadcasts to all other modules on the page. |


### Complete context usage example

```
src/context/app_context.go       ← 1. define the struct
src/components/counter/          ← 2. writer component
src/components/sidebar/          ← 3. reader component
src/pages/home/home.go           ← 4. mount the manager
```

**Step 1 — define the shared struct** in `src/context/`:

```go
package gothicwasm // any package name works

import . "github.com/felipegenef/gothicframework/pkg/wasm"

type App struct {
    GothicSharedContext `name:"app"`
    Count int
    Theme string
    Label string
}
```

Run `gothicframework wasm` — the CLI generates `AppContext()` and a manager WASM binary.

**Step 2 — writer component** (sets state):

```go
ClientSideState: func() {
    ctx := AppContext()   // *AppContext

    CreateWasmFunc("increment", func() {
        // ctx.Set fans out to per-field set-requests via the manager
        ctx.Set(App{
            Count: ctx.Count.Peek() + 1,
            Theme: ctx.Theme.Peek(),
            Label: ctx.Label.Peek(),
        })
    })

    // Or set a single field directly — even more efficient:
    CreateWasmFunc("toggleTheme", func() {
        if ctx.Theme.Peek() == "light" {
            ctx.Theme.Set("dark")   // only the Theme field is broadcast
        } else {
            ctx.Theme.Set("light")
        }
    })
},
```

**Step 3 — reader component** (reacts to state):

```go
ClientSideState: func() {
    ctx := AppContext()   // same key → same manager → same state

    Observe(func() {
        SetText("count-display", strconv.Itoa(ctx.Count.Get()))
    }, ctx.Count)   // only re-runs when Count changes, not on Theme/Label updates

    Observe(func() {
        if ctx.Theme.Get() == "dark" {
            AddClass("body", "dark-mode")
        } else {
            RemoveClass("body", "dark-mode")
        }
    }, ctx.Theme)
},
```

**Step 4 — mount the manager** once in your page template:

```go
// home.go
templ Home() {
    @ContextManagerComponent(App{})   // boots the manager WASM
    @CounterComponent()
    @SidebarComponent()
}
```

The manager WASM must be on the page before any consumer calls `AppContext()`. `ContextManagerComponent` handles this automatically.

### Lower-level key factories (advanced)

For one-off primitive shares without defining a struct, the runtime exposes typed key factories. These are what the generated context code uses under the hood.

**Primitives** — lightweight, no extra binary cost:

| Factory | Type |
|---------|------|
| `BoolKey(name)` / `StringKey(name)` | `ContextKey[bool]` / `ContextKey[string]` |
| `IntKey(name)`, `Int8/16/32/64Key(name)` | signed-int families |
| `UintKey(name)`, `Uint8/16/32/64Key(name)` | unsigned-int families |
| `Float32Key(name)` / `Float64Key(name)` | `ContextKey[float32/64]` |
| `RuneKey(name)` (= int32) / `ByteKey(name)` (= uint8) | aliases |

**Binary** — bespoke codec, smallest payload:

| Factory | Type |
|---------|------|
| `BinaryKey[T any](name, encode, decode)` | `ContextKey[T]` |
| `AutoKey[T any](name)` | `ContextKey[T]` — placeholder rewritten at build time to a `BinaryKey` with auto-generated encode/decode |

`AutoKey` is the recommended path: the CLI generates the encoder/decoder for `T` automatically. The `BinaryKey` form is only needed when you want hand-rolled codecs.

---

### How it works — full communication flow

Each WASM module runs in its own Go heap — `*Observable[T]` pointers cannot cross module boundaries. The generated context system uses a **manager WASM** as the single source of truth and broadcasts per-field binary updates to consumer WASMs through the JS event bus.

```
  Consumer WASM A              Manager WASM              Consumer WASM B
  (e.g. counter.wasm)       (e.g. page-ctx-mgr.wasm)    (e.g. sidebar.wasm)
  ─────────────────────     ─────────────────────────    ─────────────────────
  ctx.Theme.Set("dark")
    │
    │  encode field → []byte
    │  RequestCtxSetField(
    │    "page", "Theme",
    │    string(bytes))
    │
    ▼
  ── JS event bus ──────────────────────────────────────────────────────────▶
  gothic:ctx-req:page:Theme
                            │
                            │  _fields["Theme"] = bytes
                            │  BroadcastCtxEncodedField(
                            │    "page", "Theme",
                            │    string(bytes))
                            │
                            ▼
  ◀── JS event bus ──────────────────────────────────────────────────────────
  gothic:context:page:Theme        gothic:context:page:Theme
    │                                │
    │  decode bytes → string         │  decode bytes → string
    │  ctx.Theme.ApplyExternal(v)    │  ctx.Theme.ApplyExternal(v)
    │  → Observe callbacks fire      │  → Observe callbacks fire
    ▼                                ▼
  DOM updated                      DOM updated


  ctx.Set(Page{...})   ← whole-struct fan-out path
    │
    │  encode struct → []byte
    │  RequestCtxSet("page", string(bytes))
    │
    ▼
  ── JS event bus ──────────────────────────────────────────────────────────▶
  gothic:ctx-req:page
                            │
                            │  _captureAllFields(bytes)   ← zero-alloc scan
                            │  for each field:
                            │    nb = _captureField(d)    ← raw wire bytes
                            │    if !_bytesEqual(nb,      ← diff check
                            │         _fields[field]):
                            │      _fields[field] = copy(nb)
                            │      BroadcastCtxEncodedField(...)
                            │      _wholeDirty = true
                            │  if _wholeDirty:
                            │    UpdateCtxOnlineStore(...)  ← updates JS map,
                            │                               no event dispatch
                            ▼
  ◀── JS event bus ─────────────────────── (only changed fields broadcast)


  New consumer boots
    │
    │  ReadCtxStore("page") ← reads window.__gothic_context map
    │    → returns last whole-struct bytes (kept fresh by
    │      UpdateCtxOnlineStore on every mutation)
    │  decode → apply all fields  ← hydrated from store
    │  ctx._online = true
    │
    │  PingUntilOnline(...)  ← if store was empty, ping manager
    │    → manager responds with _broadcastOnline()
    │       → gothic:ctx-online:page
    │         → ListenCtxOnline fires, full hydration
    ▼
  online, reactive
```

**Two JS stores** serve different roles (see the table in the `dispatchDirect` section for full details):
- `window.__gothic_ctx` — binary buffer manager, keyed by full event name. Feeds event listeners via `CopyBytesToGo`.
- `window.__gothic_context` — string store keyed by short key name. Fed by `UpdateCtxOnlineStore` / `BroadcastCtxOnline`. Read by `ReadCtxStore` in consumer constructors — no event needed, just a direct map lookup. The manager keeps it fresh on every mutation so late-joining consumers never see stale data.

**`dispatchHold`** is a Go-side `map[string][]byte` that keeps each payload slice alive until the next dispatch on the same key overwrites it — preventing the GC from collecting the buffer while the async microtask is queued but not yet fired.

From the consumer's perspective each field is an ordinary observable — subscribe to it in `Observe`, read it with `.Get()`.

---

### JS bridge internals and known constraints

This section documents two root-cause bugs that were found and fixed inside the Gothic runtime. You do not need to understand these to use the API, but they explain why the runtime is structured the way it is and what to watch for if you upgrade TinyGo.

#### Problem 1 — Re-entrant `go._resume()` (asyncify scheduler corruption)

`document.dispatchEvent` is **synchronous**: it fires all listeners before returning. When Go calls `dispatchEvent` from inside a goroutine, the listener callback re-enters the TinyGo asyncify scheduler (`go._resume()`) while `exports.resume()` is already on the JS call stack. That double-entry corrupts the scheduler state and eventually causes a `RuntimeError: unreachable`.

```
WITHOUT the fix:
  User click
    → exports.resume()        ← scheduler starts
      → Go calls dispatchEvent
        → listener fires synchronously
          → go._resume()      ← called AGAIN while already running → crash
```

**Fix (Gothic-side, no TinyGo patch needed):** the runtime calls `window.__gothicDispatchAsync(eventName)` instead of `document.dispatchEvent` directly. That helper, injected by Gothic's bootstrap script, defers the dispatch via `queueMicrotask` so it fires only after `exports.resume()` has returned:

```
WITH the fix:
  User click
    → exports.resume() runs and returns  ← stack is clean
  ── microtask queue drains ──
    → dispatchEvent fires
      → listener → exports.resume()      ← safe, nothing else on the stack
```

`queueMicrotask` is the right primitive here: it fires at the earliest safe moment (after the current call stack unwinds, before any other user events), with no added latency.

---

#### Problem 2 — `_values[]` table growth (TinyGo lacks JS finalizers) — fully fixed

TinyGo's JS bridge maintains a `_values[]` array that maps integer ids to live JS objects. Every `js.Value` returned by `New()`, `.Get()`, or `.Call()` on an object adds a slot. Because TinyGo has no `runtime.SetFinalizer`, those slots are **never freed** when the Go-side `js.Value` goes out of scope.

Three independent leaks were found and fixed:

**Leak A — `__gothic_ctx.set()` Uint8Array (context dispatch path)**

The original `jsUint8ArrayFromBytes`-per-broadcast path allocated a brand-new `Uint8Array` on every context broadcast, creating a permanent `_values[]` entry each time. An intermediate fix (`dispatchDirect`) eliminated that by passing a raw WASM memory offset so the JS side reads from `instance.exports.memory.buffer` directly, with no `Uint8Array` passed through the TinyGo bridge at all. A residual leak remained in the bootstrap's `.slice()` call, which created a new `Uint8Array` on every broadcast. That is now fixed: the bootstrap maintains a persistent per-key `ArrayBuffer`/`Uint8Array` pair that grows with pure-doubling capacity (`byteLen < 128 ? 128 : byteLen * 2`). The `_values[]` entry for a given key is created once at first use and never replaced unless the payload grows past the current buffer capacity.

**Leak B — `findScope()` MouseEvent boxing**

`findScope()` in `events.go` called `js.Global().Get("event")` on every click, which boxes the live `MouseEvent` into a `_values[]` slot. Because TinyGo only calls `finalizeRef` for string values, the `MouseEvent` slot was never freed. The fix adds a `window.__gothicFindScope` JS helper (injected by the bootstrap) that performs the DOM walk and returns only the scope ID as a plain string. The Go side calls it and immediately discards the result with `.String()`, which triggers `finalizeRef` and frees the slot.

**Leak C — `PingCtxManager` CustomEvent allocation**

`PingCtxManager` in `context.go` allocated a new `CustomEvent` on every ping for every context key. The fix caches one `CustomEvent` per key in a `var pingEvents = map[string]js.Value{}` map. The slot is created once on the first ping for a key and reused for all subsequent pings.

**Root cause of all three:** TinyGo's `wasm_exec.js` only invokes `finalizeRef` (the slot-reclaim path) for string-typed values. Every other JS object type — `Uint8Array`, `MouseEvent`, `CustomEvent` — occupies a permanent `_values[]` slot.

**Post-fix expectation:**

| Leak | Before | After |
|------|--------|-------|
| `__gothic_ctx.set()` Uint8Array | ~36 MB/click at 150 k items | 0 new `_values[]` entries (stable payload); O(log N) per key (growing payload) |
| `findScope()` MouseEvent | ~500 B/click | 0 new `_values[]` entries per click |
| `PingCtxManager` CustomEvent | N entries (N = context keys × pings) | 1 entry per context key, at first ping only |

---

#### TinyGo unsigned-pointer bug — current workaround and planned Gothic-side fix

TinyGo's wasm_exec.js bridge passes Go pointers as signed i32 across the JS boundary. If the Go heap grows past 2 GiB, any pointer above that threshold becomes a negative integer in JavaScript, and `new Uint8Array(buffer, negativeOffset, len)` throws a `RangeError`. This affects `loadSlice`, `loadString`, `copyBytesToGo`, `copyBytesToJS`, `random_get`, and `fd_write`.

**Current workaround:** Gothic ships a patched `wasm_exec.js` (in `pkg/data/wasm_exec/`) with `>>>= 0` unsigned-right-shift coercions applied at each affected site. A drift test (`pkg/helpers/wasm/wasm_exec_drift_test.go`) compares the sha256 of the live TinyGo install against the recorded original hash; it fails if TinyGo is upgraded without re-applying the patches.

**Why this is unsatisfying:** every TinyGo upgrade requires manually re-applying patches, updating the metadata sha256, and re-running the drift test. A PR to fix this upstream in TinyGo has been prepared, but until it merges Gothic must maintain its own copy.

---

#### Implemented: direct WASM memory transport

The root cause of both the `copyBytesToJS`/`copyBytesToGo` patch requirement AND the residual `_values[]` growth was that the dispatch path crossed the JS bridge with Go-heap pointers. That bridge is now bypassed for payload transfer.

**Core idea:** Go passes payload as a raw byte-offset integer (a `uintptr` cast to `int32` — always non-negative in WASM's 32-bit address space). Gothic's own bootstrap JS reads from `instance.exports.memory.buffer` directly using that offset. No TinyGo bridge functions are involved for the payload, so no unsigned-pointer issue and no `_values[]` entries for the payload bytes.

---

**`pkg/helpers/routes/wasm_bootstrap.go`**

The generated bootstrap script injects `window.__gothic_ctx`, `window.__gothicDispatchAsync`, and `window.__gothicFindScope` under separate guards. Each WASM module also registers a per-instance entry in `window.__gothic_set` that captures `r.instance` in a closure — this is how `dispatchDirect` reaches `__gothic_ctx.set` with the correct instance reference without a global `__gothicInst`.

```js
// Shared broadcast buffer manager — created once, shared across all modules.
if (!window.__gothic_ctx) {
    window.__gothic_ctx = (function() {
        var _state = {};  // keyName → Uint8Array view (current payload)
        var _subs  = {};  // keyName → [handler fn]
        var _bufs  = {};  // keyName → ArrayBuffer (capacity-doubling pool)
        var _views = {};  // keyName → Uint8Array (current view into _bufs[key])
        return {
            // Called via __gothic_set[moduleID] with the raw WASM memory offset.
            set: function(keyName, ptrI32, byteLen, inst) {
                var offset = ptrI32 >>> 0;
                var src = new Uint8Array(inst.exports.memory.buffer, offset, byteLen);
                var buf = _bufs[keyName];
                if (!buf || buf.byteLength < byteLen) {
                    var cap = byteLen < 128 ? 128 : byteLen * 2;
                    buf = new ArrayBuffer(cap);
                    _bufs[keyName] = buf;
                    _views[keyName] = null;
                }
                var view = _views[keyName];
                if (!view || view.byteLength !== byteLen) {
                    view = new Uint8Array(buf, 0, byteLen);
                    _views[keyName] = view;
                }
                view.set(src);           // copy from WASM linear memory
                _state[keyName] = view;  // expose for .get()
                var handlers = _subs[keyName];
                if (handlers) {
                    handlers.forEach(function(h) {
                        queueMicrotask(function() { h(view); });
                    });
                }
            },
            subscribe: function(keyName, fn) {
                (_subs[keyName] = _subs[keyName] || []).push(fn);
            },
            get: function(keyName) { return _state[keyName] || null; }
        };
    })();
}
if (!window.__gothicDispatchAsync) {
    window.__gothicDispatchAsync = function(name) {
        queueMicrotask(function() { document.dispatchEvent(new CustomEvent(name)); });
    };
}
if (!window.__gothicFindScope) {
    // Takes no arguments — reads window.event directly, uses .closest() for
    // a single O(depth) DOM walk instead of a manual while loop.
    window.__gothicFindScope = function() {
        var e = window.event;
        if (!e || !e.target) return '';
        var el = e.target.closest('[data-gothic-scope]');
        return el ? (el.dataset.gothicScope || '') : '';
    };
}

// Per-instance dispatch shim — captures r.instance in closure so Go's
// dispatchDirect can call __gothic_ctx.set with the correct instance
// without a global __gothicInst variable.
window.__gothic_set = window.__gothic_set || {};
window.__gothic_set[id] = function(k, p, n) {
    window.__gothic_ctx.set(k, p, n, r.instance);
};
go.run(r.instance);
```

`_bufs`/`_views` implement a **capacity-doubling persistent buffer pool** per key: the `ArrayBuffer` for a key grows to `max(128, byteLen * 2)` the first time it is written and is reused for all subsequent writes of the same size. The `_values[]` entry for the key is created once at first use and never replaced unless the payload outgrows the current capacity. This is what keeps `_values[]` flat across thousands of broadcasts for stable-payload keys.

---

**`pkg/wasm/wasm-runtime/runtime/context.go`**

`dispatchDirect` stores the buffer in `dispatchHold` keyed by the **full event name** (prefix + key), then calls the per-module `__gothic_set[moduleID()]` shim which forwards to `__gothic_ctx.set` with the correct instance reference. It then queues an async dispatch via `__gothicDispatchAsync`:

```go
func dispatchDirect(keyName, eventPrefix string, encoded []byte) {
    buf := make([]byte, len(encoded))
    copy(buf, encoded)
    // Key includes eventPrefix so different event types on the same keyName
    // don't clobber each other in the hold map.
    dispatchHold[eventPrefix+keyName] = buf

    ptr := int32(uintptr(unsafe.Pointer(unsafe.SliceData(buf))))
    // __gothic_set[moduleID()] = func(k,p,n){ __gothic_ctx.set(k,p,n,r.instance) }
    // The instance reference is captured in the bootstrap closure — no global
    // __gothicInst variable exists.
    js.Global().Get("__gothic_set").Get(moduleID()).Invoke(
        js.ValueOf(eventPrefix+keyName),
        js.ValueOf(ptr),
        js.ValueOf(len(buf)),
    )
    js.Global().Call("__gothicDispatchAsync", js.ValueOf(eventPrefix+keyName))
}
```

Event listeners (e.g. `ListenCtxEventField`) call `__gothic_ctx.get(fullKey)` to read the `Uint8Array` view, copy it with `js.CopyBytesToGo(dst, data)`, then call `fn(string(dst))`. The `.String()` call triggers `finalizeRef` on the temporary `js.Value`, keeping `_values[]` bounded.

**Two JS stores — not one:**

| Object | Created by | Keys | Contains | Used by |
|--------|-----------|------|----------|---------|
| `window.__gothic_ctx` | bootstrap (JS) | full event name, e.g. `"gothic:ctx-online:page"` | `Uint8Array` view into persistent buffer | `ListenCtxEvent`, `ListenCtxOnline`, `ListenCtxEventField`, etc. |
| `window.__gothic_context` | `ensureContextStore()` (Go runtime) | short key name, e.g. `"page"` | string-encoded payload | `ReadCtxStore`, `UpdateCtxOnlineStore`, `BroadcastCtxOnline` |

`ReadCtxStore("page")` reads from `window.__gothic_context["page"]` (string).
`UpdateCtxOnlineStore("page", bytes)` writes `string(bytes)` to `window.__gothic_context["page"]`.
`ListenCtxOnline` reads from `window.__gothic_ctx["gothic:ctx-online:page"]` (binary `Uint8Array`).

---

**`pkg/data/wasm_exec/`**

The directory still exists and contains the patched `wasm_exec.js` with `>>>= 0` unsigned-right-shift coercions at every affected bridge site (`loadSlice`, `loadString`, `copyBytesToGo`, `copyBytesToJS`, `random_get`, `fd_write`). This patched copy is kept as the TinyGo bridge because the unsigned-pointer fix has not yet merged upstream. A drift test (`pkg/helpers/wasm/wasm_exec_drift_test.go`) detects TinyGo upgrades that would require re-applying the patches.

The direct-memory transport means the payload bytes themselves no longer pass through these patched bridge functions, so the practical risk of the unsigned-pointer bug is greatly reduced — but the patched `wasm_exec.js` is retained until the upstream fix lands.


---

## Complete example

```go
ClientSideState: func() {
    count := CreateObservable(0)
    step  := CreateObservable(1)

    Observe(func() {
        SetText("count-display", strconv.Itoa(count.Get()))
        SetText("total-display", strconv.Itoa(count.Get()*step.Get()))
    }, count, step)

    CreateWasmFunc("increment", func() { count.Set(count.Get() + step.Get()) })
    CreateWasmFunc("decrement", func() { count.Set(count.Get() - step.Get()) })
    CreateWasmFunc("reset", func() {
        count.Set(0)
        step.Set(1)
    })

    CreateWasmStringFunc("setStep", func(val string) {
        if n, err := strconv.Atoi(val); err == nil && n > 0 {
            step.Set(n)
        }
    })

},
```

---

## Architectural constraint: WASM32 heap exhaustion on large context payloads

This is a **known, unsolved architectural limitation** of TinyGo WASM32 + the Gothic context system. It is distinct from the `_values[]` JS-bridge leaks documented in Problem 2 above. The JS-bridge leaks have been fixed; this constraint cannot be fixed at the Gothic level.

### What happens

Each Gothic WASM module runs inside the browser as a 32-bit WebAssembly binary. WASM32 has a hard **4 GB linear memory ceiling** — the entire address space is `[0, 2³²)` bytes. Go's heap lives inside that address space. When the context system broadcasts a large payload (e.g. a deeply-nested struct with 10 k+ items), the following happens on every broadcast:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  WASM32 Linear Memory  (hard ceiling: 4 GB)                                  │
│                                                                              │
│  ┌────────────┐  ┌────────────────────────────────────────────────────────┐ │
│  │ Go runtime │  │  Go heap  (grows with each Encode call)                │ │
│  │  ~1–2 MB   │  │                                                        │ │
│  │            │  │  ┌──────────────────┐  ┌──────────────────┐           │ │
│  │            │  │  │ encoded payload  │  │ encoded payload  │   ...      │ │
│  │            │  │  │  broadcast N     │  │  broadcast N+1   │           │ │
│  │            │  │  │  ~12 MB          │  │  ~12 MB          │           │ │
│  │            │  │  └──────────────────┘  └──────────────────┘           │ │
│  │            │  │                                                        │ │
│  │            │  │  GC runs between broadcasts — heap SHRINKS back        │ │
│  │            │  │  but OS/WASM runtime does NOT return pages to the      │ │
│  │            │  │  host. High-water mark grows monotonically. ───────▶  │ │
│  └────────────┘  └────────────────────────────────────────────────────────┘ │
│                                                              ▲               │
│                                        committed pages never │ released      │
└──────────────────────────────────────────────────────────────────────────────┘
```

Go's GC reclaims allocations between broadcasts, so the **live set** stays flat. But the underlying WASM linear memory is **never returned to the host** — pages are committed on demand and held forever. Chrome reports this committed memory as process RSS. After enough large broadcasts the high-water mark approaches 4 GB and the Go allocator starts failing, causing a WASM `unreachable` trap.

### The broadcast pipeline (per-field)

Every call to `ctx.Set(largeStruct)` triggers this pipeline:

```
Go (consumer WASM)                       Manager WASM               JS (browser)
──────────────────────────────────────   ────────────────────────   ─────────────
1. Encode largeStruct → []byte
   (binary codec, ~12 MB for 10 k items)
   ↓ ALLOCATES on Go heap (manager side)

2. RequestCtxSet(key, bytes) ──────────▶ 3. _captureAllFields(bytes)
                                            zero-alloc scan via _skip_*
                                            diff each field vs _fields[]
                                            for CHANGED fields only:
                                              BroadcastCtxEncodedField
                                              ↓ dispatchDirect (per-field)
                                              ↓ __gothic_set[id].Invoke ──▶ __gothic_ctx.set
                                              ↓ __gothicDispatchAsync ────▶ CustomEvent fires
                                            UpdateCtxOnlineStore
                                            (updates __gothic_context,
                                             NO gothic:ctx-online event)

4. Consumer ListenCtxEventField fires      (only for CHANGED fields)
   dst = CopyBytesToGo(fieldBytes)
   → decode just that field  ←────── allocation proportional to
   ApplyExternal(v)                   ONE field, not the full struct
   Observe callbacks fire
   DOM updated
```

**Per click: allocates approximately `changedFieldSize × numSubscribers`** — not `fullStructSize × numSubscribers`. For a click that bumps a counter (`int` = 4 bytes), the consumer allocates ~4 bytes, not 12 MB, even when a 10k-item list is part of the same struct.

**Full-struct allocation still happens:**
- In the manager WASM on the initial `_captureAllFields` scan (zero-alloc pointer walk — no Go objects)
- In every consumer on **ping responses** only (`ListenCtxOnline` fires, `incoming := []byte(detail)` = full struct bytes). Pings are rare — once on boot and whenever a new consumer mounts.

### Why pages are never returned

WASM linear memory grows via `memory.grow(numPages)`. There is **no `memory.shrink`** instruction in the WASM spec. Once a page is committed it is committed for the lifetime of the module instance. Go's runtime calls `memory.grow` and manages its own heap inside that space, but it cannot hand pages back to the browser.

```
Chrome process RSS
│
│              ●                   ●
│         ●        ●          ●        ●
│    ●         ●        ●  ●       ●
│  ●
│●
└───────────────────────────────────────▶ time / broadcasts
  Each broadcast ratchets the high-water
  mark UP. GC reduces live objects but
  NOT committed pages. RSS never decreases.
```

### Failure mode

When the Go allocator exhausts the 4 GB WASM32 address space it panics with a `runtime: out of memory` trap, which surfaces in the browser as:

```
RuntimeError: unreachable
  at wasm-function[…] (wasm_exec.js)
  at syscall/js.valueCall (wasm_exec.js:…)
```

All goroutines in the module are killed. The WASM instance is unrecoverable; the page must be hard-refreshed.

### Why the JS-bridge leak fixes do NOT help here

The `_values[]` fixes (Problem 2 above) address a different layer: they eliminate permanent JS-heap allocations caused by TinyGo's bridge. Those fixes keep `window.__gothicGo._values.length` flat and reduce Chrome JS heap, but they do **not** reduce the Go-heap allocations inside WASM linear memory. Both problems cause Chrome memory growth, but through completely different mechanisms:

| Symptom | Layer | Fixed? |
|---------|-------|--------|
| `_values[]` grows without bound | JS heap (V8) | ✅ Yes — Problem 2 fixes |
| Chrome RSS grows after large broadcasts | WASM32 linear memory (Go heap) | ❌ No — architectural |

### Why WASM64 does NOT solve this

A common first instinct is "just use WASM64 — bigger address space, problem gone." That is wrong. WASM64 raises the ceiling from 4 GB to 16 exabytes, but the memory still grows monotonically on every large broadcast. On a machine with 8 GB of RAM you would still OOM the user's OS — you would just hit the machine wall instead of the WASM wall. The failure mode is identical; only the threshold changes. WASM64 is a delay, not a fix.

### How Gothic mitigates this today

Two mitigations are implemented:

**Per-field subscriptions** — instead of broadcasting the full struct on every mutation, each field gets its own event key (`gothic:context:<key>:<field>`). A module observing only `Theme` allocates only Theme's wire bytes per click. The full struct only crosses the bridge on ping responses (boot hydration), not per click.

**Design constraint** — context is designed for UI state: selected tab, theme, user info, feature flags — payloads in bytes to low kilobytes. If your encoded context payload exceeds ~100 kB, treat it as a design smell. Split the struct, paginate, or move large data to a server-side endpoint. At 100 kB and below, the heap-pressure ratchet is slow enough that normal page navigation (which unloads and resets the WASM module) prevents any real accumulation.

### What to watch for in your app

If you use the context system with large structs, monitor Chrome Task Manager's "Memory" column during development. If it climbs monotonically with each context broadcast, you are hitting this constraint. The `_values[]` counter is **not** a useful signal here — it will stay flat (the JS-bridge leaks are fixed), while RSS still grows.

The `unreachable` trap is the dramatic end-state; the subtler version — Chrome RSS climbing to 2–3 GB and slowing down — happens well before the crash and is the real user-facing problem in long-running sessions.

---

## Per-field context architecture

Gothic uses per-field subscriptions combined with two further refinements that
are load-bearing for stress workloads. This section documents the shipped behaviour.

### The context manager WASM is the sole writer

Each `ContextKey` in `src/context/` produces a dedicated **manager WASM** built
from `wasm_ctx_manager_main.go.tmpl`. It is mounted once per page via
`ContextManagerComponent(...)`. The manager:

- Owns the canonical encoded state for that key as `_lastWholeEncoded` plus a
  `map[string][]byte` of per-field byte slices (`_fields`).
- Listens to **per-field** set-requests from consumer pages via
  `ListenCtxSetReqField(key, field, fn)`. On each one it writes
  `_fields[field] = b`, re-broadcasts the field event, and marks `_wholeDirty`.
- Listens to **whole-struct** set-requests via `ListenCtxSetReq(key, fn)` —
  used by `ctx.Set(struct)` fan-out. It runs a **zero-allocation diff loop**
  (see below) to broadcast only changed fields, then updates the JS store.
- Broadcasts the whole-struct online ack on `ListenCtxPing` and at boot.
  Consumers only get a full `gothic:ctx-online` event on pings; per-mutation
  traffic is per-field events only.

Consumer pages never write canonical state directly — they always dispatch a
`RequestCtxSetField` (or the whole-struct fan-out from `ctx.Set`) and wait for
the manager's broadcast to come back through `ApplyExternal`.

### `ListenCtxSetReq` diff loop — zero-allocation field comparison

When `ctx.Set(struct)` is called by a consumer, it encodes the whole struct and
sends it as `gothic:ctx-req:<key>`. The manager's handler must decide which
fields actually changed and broadcast only those. The naive approach — decode
the full struct then re-encode each field — allocates O(N) objects for every
large slice field on every click and causes WASM heap exhaustion under stress.

The v2.16.0 approach uses **`_capture*` helpers** instead:

```
Incoming whole-struct bytes
─────────────────────────────────────────────────────────────────────
  d := &Decoder{Buf: incoming}

  Field 1: nb = _capturePings(d)       ← advances d.Pos past []Item
                                          returns sub-slice, zero alloc
           _bytesEqual(nb, _fields["Pings"])  ← raw byte compare
           → unchanged: skip broadcast

  Field 2: nb = _captureTheme(d)       ← advances d.Pos past string
           _bytesEqual(nb, _fields["Theme"])
           → CHANGED: copy + broadcast "gothic:context:page:Theme"
           _wholeDirty = true

  Field 3: nb = _captureLabel(d)       ...
  ...

  if _wholeDirty:
    _ensureWholeFresh()             ← lazy rebuild of _lastWholeEncoded
    UpdateCtxOnlineStore("page",    ← update JS map, NO event dispatch
      _lastWholeEncoded)
─────────────────────────────────────────────────────────────────────
```

Each `_capture<FieldName>(d *Decoder) []byte` helper:
- Advances `d.Pos` past the field's wire bytes using **skip helpers** (`_skip_StructName`) for struct/slice/map types — pure pointer arithmetic, no allocations.
- Returns a sub-slice of `incoming` pointing at that field's bytes. No copy, no decode.

`_bytesEqual` compares two byte slices without importing `bytes`:

```go
func _bytesEqual(a, b []byte) bool {
    if len(a) != len(b) { return false }
    for i := range a { if a[i] != b[i] { return false } }
    return true
}
```

`_skip_<Name>` advances the decoder past one encoded value of type `<Name>` without allocating a Go struct. Used internally by `_capture*` to walk `[]Item` fields in O(N) pointer arithmetic instead of O(N) allocations.

### `UpdateCtxOnlineStore` — store refresh without event dispatch

`_broadcastOnline()` does two things: updates the JS `window.__gothic_context` string store AND dispatches `gothic:ctx-online:<key>` (which updates `window.__gothic_ctx` binary buffer via `__gothic_set`). The dispatch triggers `ListenCtxOnline` in **every running consumer WASM**, which allocates the full encoded struct bytes (`incoming := []byte(detail)`) — hundreds of KB for large structs — on every click.

Before v2.16.0, `ListenCtxSetReq` called `_broadcastOnline()` on every click to fix a startup race (T5: late-joining consumer reads stale store). This caused heap exhaustion under 600-click stress tests.

The fix: `UpdateCtxOnlineStore` updates only the JS map, without the event:

```go
// manager template — ListenCtxSetReq end
if _wholeDirty {
    _ensureWholeFresh()
    UpdateCtxOnlineStore("{{.KeyName}}", _lastWholeEncoded)
}
```

```
                     ┌─ _broadcastOnline() ─────────────────────────────┐
                     │  Updates JS store ✓                              │
                     │  Dispatches gothic:ctx-online → consumers alloc  │
                     │  full struct on every click  ✗ (heap pressure)  │
                     └──────────────────────────────────────────────────┘

                     ┌─ UpdateCtxOnlineStore() ─────────────────────────┐
                     │  Updates JS store ✓                              │
                     │  No event dispatch → consumers NOT triggered     │
                     │  on clicks — only on pings  ✓                   │
                     └──────────────────────────────────────────────────┘

  Late-joining consumer:  ReadCtxStore() → reads JS map → fresh data ✓
  Ping path:              ListenCtxPing → _broadcastOnline() → full hydration ✓
```

T5 (startup race) is fully covered because `ReadCtxStore` reads the same JS map that `UpdateCtxOnlineStore` writes. Consumers that arrive after any mutation will always read the latest state.

### Per-field vs whole-struct dispatch paths

| Trigger | Wire event | Direction | Handler |
|---------|-----------|-----------|---------|
| `ctx.Theme.Set("dark")` | `gothic:ctx-req:<key>:<field>` | consumer → manager | `ListenCtxSetReqField` |
| `ctx.Set(struct)` | `gothic:ctx-req:<key>` | consumer → manager | `ListenCtxSetReq` (diff loop) |
| Manager broadcasts changed field | `gothic:context:<key>:<field>` | manager → all consumers | `ListenCtxEventField` |
| Manager ping response / boot | `gothic:ctx-online:<key>` | manager → all consumers | `ListenCtxOnline` |
| Consumer needs hydration | `gothic:ctx-ping:<key>` | consumer → manager | `ListenCtxPing` |
| Late-joining consumer | `ReadCtxStore("key")` | reads JS map directly | (no event) |

```
Consumer                  JS event bus              Manager
──────────────────────────────────────────────────────────────────
ctx.Theme.Set("dark")
  │ encode "dark" → bytes
  │ RequestCtxSetField ──▶ gothic:ctx-req:page:Theme ──▶ store bytes
  │                                                      broadcast field
  │ ◀─────────────── gothic:context:page:Theme ◀─────── (only if changed)
  │ decode bytes
  │ ApplyExternal("dark")
  │ Observe callbacks fire
  ▼
DOM updated

ctx.Set(Page{...})
  │ encode whole struct → bytes
  │ RequestCtxSet ──────▶ gothic:ctx-req:page ──────▶ _captureAllFields
  │                                                    diff each field
  │                                                    for changed fields:
  │ ◀──── gothic:context:page:Theme ◀────────────────   BroadcastField
  │ ◀──── gothic:context:page:Count ◀────────────────   BroadcastField
  │                                                    UpdateCtxOnlineStore
  ▼                                                    (no gothic:ctx-online)
DOM updated

New page load
  │ ReadCtxStore("page") ─────────────────────────▶ JS map lookup
  │ ◀──────────────── whole-struct bytes ◀─────────  (always fresh)
  │ decode → apply all fields
  │ ctx._online = true
  │ — OR if store empty —
  │ PingUntilOnline ──────▶ gothic:ctx-ping:page ──▶ _broadcastOnline()
  │ ◀──── gothic:ctx-online:page ◀───────────────────  full hydration
  ▼
online
```

`<field>` is always the literal Go field name (`Pings`, `Theme`, `Image5MB`) —
no case transformation.

### `BeginBatch` / `EndBatch` — coalescing big-struct hydration

`pkg/wasm/wasm-runtime/runtime/scheduler.go` (and its `_stub.go` mirror) expose
two functions:

```go
BeginBatch()  // suppress Observe notifications
EndBatch()    // flush a single coalesced notification
```

The generated consumer template (`wasm_page_main.go.tmpl`) wraps every field's
`ApplyExternal` inside its `ListenCtxOnline` handler in a single
`BeginBatch()` / `EndBatch()` pair. Without batching, hydrating a 39-field
struct fired 39 separate Observe notifications, each one re-running every
subscriber's callback. With batching, the page sees one coalesced reactive
update for the entire struct — a critical perf win when a single context push
must not ratchet the WASM heap.

### Manager-side lazy rebuild (`_wholeDirty`)

The first iteration of the per-field manager eagerly called `_rebuildWhole()`
after every `ListenCtxSetReqField` so `_lastWholeEncoded` always reflected the
latest state. Under stress workloads — random clicks that include a 5 MB image
field — this allocated a fresh ≥5 MB buffer on
every click. TinyGo wasm32's GC could not keep up and the run crashed with
`unreachable` after ~150 clicks.

The fix has three parts:

1. **`_wholeDirty` flag.** Per-field SetReq handlers no longer rebuild — they
   only mark the whole struct dirty. The full concatenation runs only when a
   read path (currently `_broadcastOnline`, fired on ping/online) calls
   `_ensureWholeFresh()`.
2. **Re-anchor `_fields[]` on rebuild.** After `_rebuildWhole` allocates the
   new concatenated buffer, it calls `_captureAllFields(_lastWholeEncoded)` so
   every per-field slice points back into the new buffer. Without this step,
   `_fields[Image5MB]` would still reference whichever older buffer it last
   came in on, keeping that 5 MB blob alive in parallel with the new
   concatenation.
3. **Zero-copy whole-struct ingest.** `ListenCtxSetReq` stores the incoming
   payload as `_lastWholeEncoded` and slices it directly into `_fields[]`. No
   re-encoding; both the canonical whole-struct buffer and every field slice
   share the same underlying allocation.

After these three changes, the codec stress suite survives 30 s of random
clicks (including 5 MB image presses) with zero `unreachable` traps.
