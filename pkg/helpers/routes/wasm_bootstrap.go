package helpers

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
)

// wasm_bootstrap.go isolates the HTML/JS surface used to bootstrap a WASM module
// for a server-rendered route. It is intentionally kept tiny so it can be tested
// without spinning up the rest of the routes package.
//
// Duplicate-component disambiguation:
// Two renders of the same component (same wasmName) used to emit the SAME
// data-gothic-wasm attribute. The client-side bootstrap then resolved every
// instance to the FIRST matching wrapper in the DOM via
// document.querySelector('[data-gothic-wasm="..."]:not([data-gothic-scope])'),
// so two PingMirror instances on one page ended up sharing state. We now stamp
// each render with a unique data-gothic-inst="<hex>" and thread that ID into
// the bootstrap script so the JS selector targets one specific wrapper. The
// ID is opaque — only the JS that ships with the same render consumes it, so
// the wire format is unchanged.

// newInstanceID returns a per-render opaque identifier used to disambiguate
// multiple wrappers that share the same wasmName on the same page.
func newInstanceID() string {
	return strconv.FormatUint(uint64(rand.Uint32()), 16)
}

// injectGothicScope marks the scope boundary for a WASM instance and stamps a
// per-render instance id on the wrapper. The instance id is returned so the
// bootstrap script can be wired to this specific wrapper.
//
// Returns: (modifiedHTML, instanceID).
func injectGothicScope(html []byte, wasmName string) ([]byte, string) {
	inst := newInstanceID()
	attr := `data-gothic-wasm="` + wasmName + `" data-gothic-inst="` + inst + `"`
	if bytes.Contains(html, []byte("<body")) {
		return bytes.Replace(html, []byte("<body"), []byte("<body "+attr), 1), inst
	}
	var buf bytes.Buffer
	buf.WriteString(`<div ` + attr + ` style="display:contents">`)
	buf.Write(html)
	buf.WriteString(`</div>`)
	return buf.Bytes(), inst
}

// injectWasmBootstrap injects the WASM loader script. The instance id passed in
// is baked into the JS selector so each render attaches its module to its own
// wrapper, not the first matching wrapper on the page.
//
// The scope ID (data-gothic-scope) is still generated client-side via
// Math.random() so it remains uncorrelated across page lifecycles; the
// instance id is only used to *find* the right wrapper, not to identify the
// module on the global registry.
func injectWasmBootstrap(html []byte, wasmName string, compression CompressionMethod, inst string) []byte {
	isFullPage := bytes.Contains(html, []byte("</body>"))

	// For full pages there is exactly one <body>, so no disambiguation is
	// required — the instance id is unused but still emitted on <body> for
	// consistency. For fragments we use the instance id to look up the wrapper.
	var findEl string
	if isFullPage {
		findEl = `document.querySelector('body[data-gothic-wasm="` + wasmName + `"]')`
	} else {
		findEl = `(document.currentScript&&document.currentScript.previousElementSibling)` +
			`||document.querySelector('[data-gothic-wasm="` + wasmName + `"][data-gothic-inst="` + inst + `"]')`
	}

	ext := ".wasm.gz"
	if compression == BROTLI {
		ext = ".wasm.br"
	}

	// Each instance's scope id is passed BOTH via the legacy
	// window.__gothicCurrentModule global AND via Go.argv. The argv channel
	// is the deterministic one: TinyGo's wasm_exec sets os.Args from argv
	// BEFORE the WASM module's package init() runs, so each Go runtime can
	// read its own scope id without racing other in-flight bootstraps. The
	// global is kept for backwards compatibility with any third-party code
	// that may have read it, but the runtime now prefers argv.
	//
	// Format: argv[1] = "GOTHIC_SCOPE=<id>". A prefix is used so future
	// metadata can be threaded through without re-versioning.
	script := fmt.Sprintf(`<script>
(function(){
    var wn='%s';
    var el=(%s);
    if(!el)return;
    var id=wn+'-'+(Math.random()*0xFFFFFFFF>>>0).toString(16).padStart(8,'0');
    el.setAttribute('data-gothic-scope',id);
    if(!window.__gothic_ctx){
        window.__gothic_ctx=(function(){
            var _state={};
            var _subs={};
            return{
                set:function(keyName,ptrI32,byteLen,inst){
                    var offset=ptrI32>>>0;
                    var copy=new Uint8Array(inst.exports.memory.buffer,offset,byteLen).slice();
                    _state[keyName]=copy;
                    var handlers=_subs[keyName];
                    if(handlers){handlers.forEach(function(h){queueMicrotask(function(){h(copy);});});}
                },
                subscribe:function(keyName,fn){(_subs[keyName]=_subs[keyName]||[]).push(fn);},
                get:function(keyName){return _state[keyName]||null;}
            };
        })();
    }
    if(!window.__gothicDispatchAsync){
        window.__gothicDispatchAsync=function(name){
            queueMicrotask(function(){document.dispatchEvent(new CustomEvent(name));});
        };
    }
    (async function(){
        if(typeof Go==='undefined'){
            await new Promise(function(res,rej){
                var s=document.createElement('script');
                s.src='/public/wasm_exec.js';
                s.onload=res;s.onerror=rej;
                document.head.appendChild(s);
            });
        }
        var go=new Go();
        window.__gothicGo=go;
        go.argv=['gothic','GOTHIC_SCOPE='+id];
        var r=await WebAssembly.instantiateStreaming(
            fetch('/public/wasm/'+wn+'%s'),go.importObject
        );
        window.__gothicCurrentModule=id;
        window.__gothic_set=window.__gothic_set||{};
        window.__gothic_set[id]=function(k,p,n){window.__gothic_ctx.set(k,p,n,r.instance);};
        go.run(r.instance);
    })();
})();
</script>`, wasmName, findEl, ext)

	if isFullPage {
		return bytes.Replace(html, []byte("</body>"), []byte(script+"</body>"), 1)
	}
	return append(html, []byte(script)...)
}

// injectWasmEnvelope is a convenience helper that owns the instance id for one
// render: it stamps the wrapper, then bakes the same id into the bootstrap.
// Callers (wasmInjectedComponent.Render, ContextManagerComponent) should use
// this so the two halves of the envelope cannot drift.
func injectWasmEnvelope(html []byte, wasmName string, compression CompressionMethod) []byte {
	scoped, inst := injectGothicScope(html, wasmName)
	return injectWasmBootstrap(scoped, wasmName, compression, inst)
}
