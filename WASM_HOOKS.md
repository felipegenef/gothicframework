# WASM Hooks Reference

All hooks are available via the dot import in any `ClientSideState` function:

```go
import . "github.com/felipegenef/gothicframework/pkg/wasm"
```

They compile as no-ops server-side and as the real reactive TinyGo implementation in the WASM binary.

> **API naming update.** This document was rewritten to match the merged API.
> If you still see references to `UseState` / `UseEffect` / `Register` in older
> material, replace them with `CreateObservable` / `Observe` / `CreateWasmFunc`
> respectively.

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

These functions expose Go callbacks to JavaScript. Gothic routes the call to the correct WASM module when multiple components with the same function name are on the same page (see `WASM_SCOPING.md`).

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

## Context

Context lets multiple WASM components share reactive state without prop drilling. Because each component is a separate WASM module with its own Go heap, values are serialized through a JavaScript store (`window.__gothic_context`) and broadcast via `CustomEvent`. The current API uses a generated context constructor per shared struct — define the struct once in `src/context/` and the CLI generates a `<Struct>Context()` factory that handles encoding, decoding, broadcast, and subscription.

### Defining a shared context

1. Create `src/context/page_context.go` (or any name) with a struct embedding `GothicSharedContext`:

   ```go
   package context

   import . "github.com/felipegenef/gothicframework/pkg/wasm"

   type Page struct {
       GothicSharedContext
       Pings int    `gothic:"page-pings"`
       Label string `gothic:"page-label"`
       Theme string `gothic:"page-theme"`
   }
   ```

   The `gothic:` tag on each field sets the key name; without it the CLI uses the field name.

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

Canonical examples in TestGothic: `src/context/PageContext.go`, `src/context/codectestctx.go`.

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

**JSON** — structs, slices, maps — serialized as JSON:

| Factory | Type |
|---------|------|
| `JsonKey[T any](name)` | `ContextKey[T]` |

`T` must be JSON-serializable. `JsonKey` pulls in `encoding/json`, which adds ~140 KB gzip to any binary that calls it. Binaries using only primitive keys (or the generated `BinaryKey`-based context structs) are unaffected.

**Binary** — bespoke codec, smallest payload:

| Factory | Type |
|---------|------|
| `BinaryKey[T any](name, encode, decode)` | `ContextKey[T]` |
| `AutoKey[T any](name)` | `ContextKey[T]` — placeholder rewritten at build time to a `BinaryKey` with auto-generated encode/decode |

`AutoKey` is the recommended path: the CLI generates the encoder/decoder for `T` automatically. The `BinaryKey` form is only needed when you want hand-rolled codecs.

---

### How it works

Each WASM module runs in its own Go heap — `*Observable[T]` pointers cannot cross module boundaries. The generated context constructor (`<Struct>Context()`) registers a `BinaryKey` for the struct, encodes the payload as raw bytes, writes it into `window.__gothic_payload[keyName].data` (a reused `Uint8Array`), fires a `CustomEvent("gothic:context:keyName")` on `document`, and listens for that event to copy the bytes back via `js.CopyBytesToGo`. From the consumer's perspective each field is an ordinary observable — subscribe to it in `Observe`, read it with `.Get()`.

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

The generated bootstrap script injects `window.__gothic_ctx`, `window.__gothicDispatchAsync`, and `window.__gothicFindScope` under a single `if (!window.__gothic_ctx)` guard. The injection is prepended inside the per-module IIFE alongside `go.argv = [...]` and `go.run(r.instance)`:

```js
if (!window.__gothic_ctx) {
    window.__gothic_ctx = (function() {
        var _state = {};
        var _subs  = {};
        return {
            // Called by Go: keyName (string), ptrI32 (raw WASM i32 offset), byteLen (int), inst (WebAssembly.Instance)
            set: function(keyName, ptrI32, byteLen, inst) {
                var offset = ptrI32 >>> 0;
                var existing = _state[keyName];
                var cap = existing ? existing.byteLength : 0;
                var needed = byteLen < 128 ? 128 : byteLen * 2;
                if (!existing || cap < byteLen) {
                    var buf = new ArrayBuffer(needed);
                    existing = new Uint8Array(buf);
                    _state[keyName] = existing;
                }
                existing.set(new Uint8Array(inst.exports.memory.buffer, offset, byteLen));
                var view = existing.subarray(0, byteLen);
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
    window.__gothicDispatchAsync = function(name) {
        queueMicrotask(function() {
            document.dispatchEvent(new CustomEvent(name));
        });
    };
    window.__gothicFindScope = function(el) {
        while (el) {
            if (el.dataset && el.dataset.gothicScope) return el.dataset.gothicScope;
            el = el.parentElement;
        }
        return "";
    };
}
```

The WASM instance is exposed as `window.__gothicInst = r.instance` just before `go.run(r.instance)` so `__gothic_ctx.set` can reference it.

---

**`pkg/wasm/wasm-runtime/runtime/context.go`**

`dispatchDirect` passes a raw WASM memory offset to `__gothic_ctx.set`. Listeners registered via `__gothic_ctx.subscribe` receive the bytes as a `Uint8Array` view and copy them into Go with `js.CopyBytesToGo`, followed by `.String()` to trigger `finalizeRef`:

```go
func dispatchDirect(keyName, eventPrefix string, encoded []byte) {
    buf := make([]byte, len(encoded))
    copy(buf, encoded)
    dispatchHold[keyName] = buf  // keep alive until microtask fires

    ptr := int32(uintptr(unsafe.Pointer(unsafe.SliceData(buf))))
    js.Global().Get("__gothic_ctx").Call("set",
        js.ValueOf(keyName),
        js.ValueOf(ptr),
        js.ValueOf(len(buf)),
        js.Global().Get("__gothicInst"),
    )
    js.Global().Call("__gothicDispatchAsync", js.ValueOf(eventPrefix+keyName))
}
```

Listeners call `js.CopyBytesToGo(dst, u8)` then `fn(string(dst))`. The `.String()` call on the result triggers `finalizeRef`, keeping `_values[]` entries bounded.

---

**`pkg/data/wasm_exec/`**

The directory still exists and contains the patched `wasm_exec.js` with `>>>= 0` unsigned-right-shift coercions at every affected bridge site (`loadSlice`, `loadString`, `copyBytesToGo`, `copyBytesToJS`, `random_get`, `fd_write`). This patched copy is kept as the TinyGo bridge because the unsigned-pointer fix has not yet merged upstream. A drift test (`pkg/helpers/wasm/wasm_exec_drift_test.go`) detects TinyGo upgrades that would require re-applying the patches.

The direct-memory transport means the payload bytes themselves no longer pass through these patched bridge functions, so the practical risk of the unsigned-pointer bug is greatly reduced — but the patched `wasm_exec.js` is retained until the upstream fix lands.

---

**Verification**

```bash
# 1. Go build (host + WASM target)
cd /home/felipe/DEV/gothic-cli
go build github.com/felipegenef/gothicframework/...
GOOS=js GOARCH=wasm go build ./pkg/wasm/wasm-runtime/runtime/...

# 2. Go tests
go test ./pkg/helpers/... ./pkg/helpers/routes/... ./pkg/wasm/... ./pkg/cli/... ./cmd/...

# 3. Rebuild TestGothic WASMs
go build -o gothicframework .
cd /home/felipe/DEV/TestGothic
rm -f .gothicCli/wasm-cache.json
rm -rf public/wasm
/home/felipe/DEV/gothic-cli/gothicframework wasm

# 4. Playwright suite
/home/felipe/DEV/gothic-cli/gothicframework hot-reload &
npx playwright test
```

**Expected stress-test outcomes:**
- `codec-stress-random.spec.ts` — passes (no unreachable after 30 s)
- `codec-bridge-leak.spec.ts` — passes (`_values[]` stays flat; payload bytes never enter the table)
- `codec-ctsetdeep-repro.spec.ts` — passes (300 iterations complete without WASM crash)

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

    // NOTE: no trailing `select {}` needed — the WASM page template emits it
    // for you. If you leave one in, the build helper strips it before
    // rendering to avoid duplication.
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

### The broadcast pipeline

Every call to `ctx.Set(largeStruct)` triggers this pipeline:

```
Go (WASM module)                         JS (browser main thread)
─────────────────────────────────────    ─────────────────────────────────────
1. Encode largeStruct → []byte           
   (binary codec, ~12 MB for 10 k items)
   ↓ ALLOCATES on Go heap
   
2. dispatchDirect(key, encoded)
   ↓ ptrI32 = int32(uintptr(ptr))
   ↓ passes raw WASM offset to JS ──────▶ 3. __gothic_ctx.set(key, ptrI32,
                                               byteLen, instance)
                                              ↓ reads inst.exports.memory
                                                .buffer[offset:offset+byteLen]
                                              ↓ copies into persistent Uint8Array
                                              ↓ notifies subscribers via
                                                queueMicrotask
                                              
4. __gothicDispatchAsync(event) ──────────▶ 5. CustomEvent fires after GC yields
   (deferred via queueMicrotask)              subscribers call CopyBytesToGo
                                              each subscriber decodes its own
                                              copy: ANOTHER ~12 MB allocation
                                              per subscriber

6. Go GC runs, reclaims encoded + decoded
   buffers.  Live set drops back to normal.
   But COMMITTED pages stay.
```

**Every broadcast round-trip allocates approximately `payloadSize × (1 + numSubscribers)`** bytes on the Go heap. With a 12 MB payload and 3 subscribers that is ~48 MB per click. Go's GC reclaims all of it, but the WASM committed-memory high-water mark climbs by ~48 MB and never comes back down.

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

### Solution analysis

None of these is trivially cheap. They are ordered from most to least impactful.

---

#### Solution 1 — Module instance recycling *(the real fix)*

When a `WebAssembly.Instance` is garbage-collected by the browser, **all of its linear memory is returned immediately** — 100 % of the committed pages, regardless of how many `memory.grow` calls were made. This is the only mechanism that can actually reclaim WASM linear memory.

Gothic already has the key structural property that makes this feasible: **context state lives in `window.__gothic_ctx` on the JS side, not inside Go.** Go modules are consumers of that state, not owners. A fresh instance re-hydrates by re-subscribing on startup.

The recycling protocol would look like this:

```
Trigger: broadcast count threshold OR estimated heap pressure
         (e.g. every N large broadcasts, or when performance.memory.usedJSHeapSize
          crosses a heuristic limit)

┌─────────────────────────────────────────────────────────┐
│  JS orchestrator (outside WASM)                         │
│                                                         │
│  1. Snapshot: read window.__gothic_ctx._state           │
│     → plain JS object, already serialised bytes         │
│                                                         │
│  2. Destroy old instance                                │
│     → let it fall out of scope / null the reference     │
│     → browser GC reclaims ALL linear memory             │
│                                                         │
│  3. Re-instantiate fresh WebAssembly.Instance           │
│     → same .wasm binary, new address space              │
│                                                         │
│  4. Restore: write snapshot back into                   │
│     window.__gothic_ctx._state                          │
│     → new instance re-subscribes on its first           │
│       scheduler tick and picks up the current state     │
└─────────────────────────────────────────────────────────┘
```

This is what a hard page-reload does, but transparent to the user and with no visible flash. The main implementation risk is the window between step 2 and step 4 — any click during that window would see a missing WASM function. That window can be made arbitrarily short (< 1 ms on a warm binary) by buffering events in JS during the swap.

**Complexity**: high — requires changes to the bootstrap script and the module lifecycle.
**Impact**: complete — resets committed memory to zero, repeatable on demand.

---

#### Solution 2 — Zero-copy / streaming decode on subscribers

Right now every subscriber does:

```
JS Uint8Array view  ──CopyBytesToGo──▶  Go []byte copy  ──decode──▶  struct
                                        (12 MB per subscriber)
```

If the codec read directly from the `Uint8Array` view through a `unsafe.Slice` pointer into WASM linear memory — without allocating a Go-side copy — subscriber allocations drop to near zero:

```
JS Uint8Array view  ──unsafe pointer──▶  decode directly from shared buffer
                                         (0 extra allocation per subscriber)
```

The sender still needs one encoded copy (to pass the raw offset to JS), but the `numSubscribers` multiplier disappears.

**Complexity**: medium — requires a streaming binary reader in the generated codec and careful lifetime management (the view must remain valid during decode).
**Impact**: reduces per-broadcast allocation by `~payloadSize × (numSubscribers - 1)`. Does not eliminate the committed-memory ratchet — just slows it significantly.

---

#### Solution 3 — Keep large data in JS, pass handles to Go

For genuinely large data (file contents, large lists, binary blobs), store it in a JS `Map` keyed by a small ID string. Go context only carries the ID. Any module that needs the data asks JS for it by ID via a small helper call.

```
Context payload:  { listId: "abc123", count: 10482, theme: "dark" }   ← ~50 bytes
JS Map:           "abc123" → Uint8Array(12 MB)                        ← never crosses bridge
```

Go never holds the 12 MB blob. The committed-memory ratchet does not trigger because the large allocation never enters the Go heap.

**Complexity**: low for new code, medium to retrofit existing context structs.
**Impact**: complete for the specific large-data case. Does not help if the large payload is genuinely needed inside Go.

---

#### Solution 4 — Per-field subscriptions

The generated `<Struct>Context()` API broadcasts the whole struct as one blob. A module that only needs `Theme` and `Pings` still decodes and allocates the entire payload including the 10k-item list it never reads.

Per-field context keys (`BinaryKey` per field, already the underlying primitive) would let each module only allocate for the fields it actually subscribes to. The 10k-item list only enters the Go heap in modules that explicitly subscribe to it.

**Complexity**: medium — API change; generated code must expose field-level keys without breaking the struct-level ergonomics.
**Impact**: reduces allocation per broadcast proportionally to how many modules skip the large fields. Does not eliminate the ratchet for modules that do need the large field.

---

#### Solution 5 — Context is not a database *(design constraint)*

The honest root cause of the stress-test scenario is a misuse of the context system. Context is designed for UI state: selected tab, theme, user info, a feature flag — payloads measured in bytes to low kilobytes. A 10k-item list belongs in a server-side paginated endpoint, a `ContextKey` holding only the current page slice, or IndexedDB for offline scenarios.

**Rule of thumb: if your encoded context payload exceeds ~100 kB, treat it as a design smell.** Split the struct, paginate, or move the large data to JS storage. At 100 kB and below, the heap-pressure ratchet is slow enough that normal page navigation (which unloads and resets the WASM module) prevents any real accumulation.

This is not "it is what it is" — it is a deliberate boundary. The context system is a reactive broadcast channel, not a shared heap. Keeping payloads small is the cheapest mitigation of all.

---

### Summary

| Solution | Eliminates ratchet? | Complexity | Status |
|----------|--------------------|-----------:|--------|
| Module instance recycling | ✅ fully | High | Not implemented |
| Zero-copy subscriber decode | Slows it (÷ numSubscribers) | Medium | Not implemented |
| Large data in JS / handles | ✅ for large-data case | Low–Medium | Available today |
| Per-field subscriptions | Partially | Medium | Not implemented |
| Keep payloads < 100 kB | ✅ in practice | Design discipline | Recommended now |

For most production Gothic apps, **Solution 5 (design discipline) + Solution 3 (JS-side storage for blobs)** is sufficient. **Solution 1 (module recycling)** is the right long-term answer for apps that genuinely need large cross-module state.

### What to watch for in your app

If you use the context system with large structs, monitor Chrome Task Manager's "Memory" column during development. If it climbs monotonically with each context broadcast, you are hitting this constraint. The `_values[]` counter is **not** a useful signal here — it will stay flat (the JS-bridge leaks are fixed), while RSS still grows.

The `unreachable` trap is the dramatic end-state; the subtler version — Chrome RSS climbing to 2–3 GB and slowing down — happens well before the crash and is the real user-facing problem in long-running sessions.

---

## Memory leak fixes — May 2026

Three `_values[]` leaks affecting the context broadcast path, the DOM scope walk, and the ping manager were identified and fixed during the v2.16.0 development cycle. For the full root-cause analysis, reproduction steps, heap profiles, and fix rationale, see:

`.claude/project-plan/MEMORY_LEAK_INVESTIGATION.md`

The short summary: TinyGo's `wasm_exec.js` bridge only reclaims `_values[]` slots for string-typed values; every other JS object type accumulates indefinitely. The fixes eliminate or cache every non-string JS object that the Gothic runtime creates on a per-event basis.

---

## Per-field context architecture (v2.16.0)

The v2.16.0 branch implements Solution 4 from the architectural analysis above
("Per-field subscriptions") and combines it with two further refinements that
turned out to be load-bearing for stress workloads. This section documents the
final shipped behaviour.

### The context manager WASM is the sole writer

Each `ContextKey` in `src/context/` produces a dedicated **manager WASM** built
from `wasm_ctx_manager_main.go.tmpl`. It is mounted once per page via
`ContextManagerComponent(...)`. The manager:

- Owns the canonical encoded state for that key as `_lastWholeEncoded` plus a
  `map[string][]byte` of per-field byte slices (`_fields`).
- Listens to **per-field** set-requests from consumer pages via
  `ListenCtxSetReqField(key, field, fn)`. On each one it writes
  `_fields[field] = b` and re-broadcasts the field event.
- Listens to **whole-struct** set-requests via `ListenCtxSetReq(key, fn)` —
  used by `ctx.Set(struct)` fan-out and by hydration. It stores the incoming
  bytes verbatim as `_lastWholeEncoded` and slices them zero-copy into
  `_fields[]` via `_captureAllFields(b)`.
- Broadcasts the whole-struct online ack on `ListenCtxPing` and at boot.
  Consumers only hit this once per mount; per-mutation traffic is per-field.

Consumer pages never write canonical state directly — they always dispatch a
`RequestCtxSetField` (or the whole-struct fan-out from `ctx.Set`) and wait for
the manager's broadcast to come back through `ApplyExternal`.

### Per-field vs whole-struct dispatch paths

| Operation                   | Wire event                              | Manager handler          |
|-----------------------------|-----------------------------------------|--------------------------|
| `field.SetBroadcast(...)`   | `gothic:ctx-req:<key>:<field>`          | `ListenCtxSetReqField`   |
| `ctx.Set(struct)`           | `gothic:ctx-req:<key>` (whole)          | `ListenCtxSetReq`        |
| Manager → consumer per-field| `gothic:context:<key>:<field>`          | `ListenCtxEventField`    |
| Manager → consumer hydrate  | `gothic:ctx-online:<key>` (whole)       | `ListenCtxOnline`        |
| Consumer → manager wake     | `gothic:ctx-ping:<key>`                 | `ListenCtxPing`          |

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
update for the entire struct — a critical perf win for the
`heap-snapshot-leak.spec.ts` scenario where a single context push must not
ratchet the WASM heap.

### Manager-side lazy rebuild (`_wholeDirty`)

The first iteration of the per-field manager eagerly called `_rebuildWhole()`
after every `ListenCtxSetReqField` so `_lastWholeEncoded` always reflected the
latest state. Under `codec-stress-random.spec.ts` workloads — random clicks
that include the 5 MB image button — this allocated a fresh ≥5 MB buffer on
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
