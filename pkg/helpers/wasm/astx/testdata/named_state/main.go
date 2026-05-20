package main

import "fmt"

type PageConfig struct {
	ClientSideState func()
}

func myFunc() {
	fmt.Println("x")
}

var Page = PageConfig{
	ClientSideState: myFunc,
}

func main() {}
