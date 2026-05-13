//go:build !js || !wasm

package runtime

func Register(name string, fn func())            {}
func RegisterInput(name string, fn func(string)) {}
func RegisterBool(name string, fn func(bool))    {}
