package main

type PageConfig struct {
	ClientSideState func()
	WasmCompression string
}

var Page = PageConfig{
	ClientSideState: func() {
		x := 1
		_ = x
	},
}

func main() {}
