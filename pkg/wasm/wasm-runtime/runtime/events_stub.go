//go:build !js || !wasm

package runtime

func CreateWasmFunc(name string, fn func())            {}
func CreateWasmStringFunc(name string, fn func(string)) {}
func CreateWasmBoolFunc(name string, fn func(bool))    {}

func CreateWasmFuncWithReturn(name string, fn func(this JSValue, args []JSValue) any) JSValue { return JSValue{} }
