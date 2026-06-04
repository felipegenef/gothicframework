package main

import (
	"fmt"

	"example.com/helpers/helpers/routes"
)

func stateFn() {
	fmt.Println("hello")
}

var Page = routes.RouteConfig{
	ClientSideState: stateFn,
	WasmCompression: "GZIP",
}

func main() {}
