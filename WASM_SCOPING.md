# WASM Module Scoping

## What changed and why

When multiple WASM modules load on the same page (e.g. a page with state + a menu
component with state + a multiselect with state), they all call `js.Global().Set(name, f)`
which dumps every registered function into `window`. The last module to load silently
overwrites any function with the same name from a previous module — no error, just broken
behavior.

Two files were changed to fix this transparently, without any changes to user code.

---

## Changed files

### `pkg/wasm/wasm-runtime/runtime/events.go`

**Before:** each `Register` call wrote directly to `window`:
```go
func Register(name string, fn func()) {
    f := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        fn()
        return nil
    })
    keep = append(keep, f)
    js.Global().Set(name, f)
}
```

**After:** functions are stored in a per-instance namespace
(`window.__gothic_registry[instanceID][name]`). A proxy is created on `window[name]`
once per function name. The proxy reads `window.event.target.closest('[data-gothic-scope]')`
at call time to find which instance triggered the event and routes to the correct module.
Same pattern applies to `RegisterInput` and `RegisterBool`.

---

### `pkg/helpers/routes/fileBasedRouting.go`

**Before:** `wasmInjectedComponent.Render` just called `injectWasmBootstrap` which
replaced `</body>` with a `<script>` block loading the `.wasm.gz` file:
```go
func (c *wasmInjectedComponent) Render(ctx context.Context, w io.Writer) error {
    var buf bytes.Buffer
    if err := c.inner.Render(ctx, &buf); err != nil {
        return err
    }
    _, err := w.Write(injectWasmBootstrap(buf.Bytes(), c.wasmName))
    return err
}

func injectWasmBootstrap(html []byte, wasmName string) []byte {
    script := fmt.Sprintf(`<script>
(async function(){
    if(typeof Go==='undefined'){
        await new Promise((res,rej)=>{
            var s=document.createElement('script');
            s.src='/public/wasm_exec.js';
            s.onload=res;s.onerror=rej;
            document.head.appendChild(s);
        });
    }
    var go=new Go();
    var r=await WebAssembly.instantiateStreaming(
        fetch('/public/wasm/%s.wasm.gz'),go.importObject
    );
    go.run(r.instance);
})();
</script>`, wasmName)
    return bytes.Replace(html, []byte("</body>"), []byte(script+"</body>"), 1)
}
```

**After:** `Render` generates a unique instance ID per render, stamps it on the HTML
via `injectGothicScope`, and passes it to `injectWasmBootstrap` which sets
`window.__gothicCurrentModule` before `go.run()` so `Register` knows which namespace
to write into.

`injectGothicScope` detects whether the HTML is a full page or a component fragment:
- Full page (`<body>` present) → adds `data-gothic-scope` attribute to the `<body>` tag
- Component fragment (no `<body>`) → wraps content in `<div style="display:contents">`
  which is invisible to CSS layout (flexbox/grid unaffected) but present in the DOM
  so `closest('[data-gothic-scope]')` can find it.

Imports added: `"crypto/rand"`, `"encoding/hex"`.

---

## How to revert

If you want to go back to the original simple one-WASM-per-page injection with no
scoping (functions go directly onto `window`, no collision protection):

### 1. Revert `pkg/wasm/wasm-runtime/runtime/events.go`

Replace the entire file content with:

```go
//go:build js && wasm

package runtime

import "syscall/js"

var keep []js.Func

func Register(name string, fn func()) {
    f := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        fn()
        return nil
    })
    keep = append(keep, f)
    js.Global().Set(name, f)
}

func RegisterInput(name string, fn func(string)) {
    f := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        val := ""
        if len(args) > 0 {
            val = args[0].String()
        }
        fn(val)
        return nil
    })
    keep = append(keep, f)
    js.Global().Set(name, f)
}

func RegisterBool(name string, fn func(bool)) {
    f := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        val := false
        if len(args) > 0 {
            val = args[0].Bool()
        }
        fn(val)
        return nil
    })
    keep = append(keep, f)
    js.Global().Set(name, f)
}
```

### 2. Revert `pkg/helpers/routes/fileBasedRouting.go`

Remove `"crypto/rand"` and `"encoding/hex"` from the import block.

Replace `Render`, `randomID`, `injectGothicScope`, and `injectWasmBootstrap` with:

```go
func (c *wasmInjectedComponent) Render(ctx context.Context, w io.Writer) error {
    var buf bytes.Buffer
    if err := c.inner.Render(ctx, &buf); err != nil {
        return err
    }
    _, err := w.Write(injectWasmBootstrap(buf.Bytes(), c.wasmName))
    return err
}

func injectWasmBootstrap(html []byte, wasmName string) []byte {
    script := fmt.Sprintf(`<script>
(async function(){
    if(typeof Go==='undefined'){
        await new Promise((res,rej)=>{
            var s=document.createElement('script');
            s.src='/public/wasm_exec.js';
            s.onload=res;s.onerror=rej;
            document.head.appendChild(s);
        });
    }
    var go=new Go();
    var r=await WebAssembly.instantiateStreaming(
        fetch('/public/wasm/%s.wasm.gz'),go.importObject
    );
    go.run(r.instance);
})();
</script>`, wasmName)
    return bytes.Replace(html, []byte("</body>"), []byte(script+"</body>"), 1)
}
```

### 3. Rebuild and reinstall

```bash
cd /path/to/gothic-cli
go build ./cmd/ ./pkg/cli/... ./pkg/helpers/... ./pkg/wasm/...
go install .
```

---

## Known limitation of the scoped solution

The dispatch proxy relies on `window.event` being set, which is true for all
user-triggered interactions (click, input, change, focus, blur). It is `undefined`
for programmatic calls from async contexts (setTimeout, Promise callbacks). In those
cases the proxy falls back to the first registered module that has the function,
which is correct when there is only one instance of a given component on the page.
