package wasmexec

import _ "embed"

//go:embed wasm_exec.js
var Shim []byte
