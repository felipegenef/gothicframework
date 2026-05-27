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
func injectWasmBootstrap(html []byte, wasmName string, compression CompressionMethod, compiler WasmCompiler, inst string) []byte {
	isFullPage := bytes.Contains(html, []byte("</body>"))

	// For full pages there is exactly one <body>, so no disambiguation is
	// required — the instance id is unused but still emitted on <body> for
	// consistency. For fragments we use the instance id to look up the wrapper.
	var findEl string
	if isFullPage {
		// After hx-boost navigation, HTMX swaps body innerHTML but not body element
		// attributes — so body still carries the previous page's data-gothic-wasm.
		// Fall back to document.body and update its attribute so the scope stamp
		// and WASM bootstrap work correctly on every navigation.
		findEl = `(document.querySelector('body[data-gothic-wasm="` + wasmName + `"]')||` +
			`(function(){var b=document.body;if(b)b.setAttribute('data-gothic-wasm','` + wasmName + `');return b;})())`
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
    if(!window.__gothic_topic){
        window.__gothic_topic=(function(){
            var _state={};
            var _subs={};
            var _bufs={};
            var _views={};
            return{
                set:function(keyName,ptrI32,byteLen,inst){
                    var offset=ptrI32>>>0;
                    var src=new Uint8Array(inst.exports.memory.buffer,offset,byteLen);
                    var buf=_bufs[keyName];
                    if(!buf||buf.byteLength<byteLen){
                        var cap=byteLen<128?128:byteLen*2;
                        buf=new ArrayBuffer(cap);
                        _bufs[keyName]=buf;
                        _views[keyName]=null;
                    }
                    var view=_views[keyName];
                    if(!view||view.byteLength!==byteLen){
                        view=new Uint8Array(buf,0,byteLen);
                        _views[keyName]=view;
                    }
                    view.set(src);
                    _state[keyName]=view;
                    var handlers=_subs[keyName];
                    if(handlers){handlers.forEach(function(h){queueMicrotask(function(){h(view);});});}
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
    if(!window.__gothicFindScope){
        window.__gothicFindScope=function(){
            var e=window.event;
            if(!e||!e.target)return'';
            var el=e.target.closest('[data-gothic-scope]');
            return el?(el.dataset.gothicScope||''):'';
        };
    }
    (async function(){
        // Each compiler ships its own wasm_exec.js that defines globalThis.Go
        // with an incompatible importObject shape (standard Go's Go has no
        // wasi_snapshot_preview1; TinyGo's does). A page that mixes compilers
        // (e.g. a TinyGo topic manager + a Golang page WASM) would otherwise
        // race on the shared Go global and instantiate one binary against
        // the wrong shim, producing
        //   "Import #1 wasi_snapshot_preview1: module is not an object..."
        // So we cache the Go class under a per-compiler slot and always
        // construct from that slot rather than reading the live global.
        var slot='%s';
        if(!window.__gothicGoClasses)window.__gothicGoClasses={};
        if(!window.__gothicGoClasses[slot]){
            // Snapshot any pre-existing Go so we can restore it after loading
            // this compiler's shim — otherwise siblings on the page would
            // observe the wrong Go via the bare global.
            var prevGo=(typeof Go!=='undefined')?Go:undefined;
            await new Promise(function(res,rej){
                var s=document.createElement('script');
                s.src='/public/%s';
                s.onload=res;s.onerror=rej;
                document.head.appendChild(s);
            });
            window.__gothicGoClasses[slot]=Go;
            if(prevGo!==undefined){try{window.Go=prevGo;}catch(_){}}
        }
        var GoCls=window.__gothicGoClasses[slot];
        var go=new GoCls();
        window.__gothicGo=go;
        go.argv=['gothic','GOTHIC_SCOPE='+id];
        var r=await WebAssembly.instantiateStreaming(
            fetch('/public/wasm/'+wn+'%s'),go.importObject
        );
        window.__gothicCurrentModule=id;
        window.__gothic_set=window.__gothic_set||{};
        window.__gothic_set[id]=function(k,p,n){window.__gothic_topic.set(k,p,n,r.instance);};
        go.run(r.instance);
    })();
})();
</script>`, wasmName, findEl, wasmExecFile(compiler), wasmExecFile(compiler), ext)

	if isFullPage {
		return bytes.Replace(html, []byte("</body>"), []byte(script+"</body>"), 1)
	}
	return append(html, []byte(script)...)
}

// wasmExecFile returns the correct wasm_exec.js filename for the given compiler.
func wasmExecFile(compiler WasmCompiler) string {
	if compiler == Golang {
		return "wasm_exec_go.js"
	}
	return "wasm_exec.js"
}

// injectWasmEnvelope is a convenience helper that owns the instance id for one
// render: it stamps the wrapper, then bakes the same id into the bootstrap.
// Callers (wasmInjectedComponent.Render, TopicManagerComponent) should use
// this so the two halves of the envelope cannot drift.
func injectWasmEnvelope(html []byte, wasmName string, compression CompressionMethod, compiler WasmCompiler) []byte {
	scoped, inst := injectGothicScope(html, wasmName)
	return injectWasmBootstrap(scoped, wasmName, compression, compiler, inst)
}
