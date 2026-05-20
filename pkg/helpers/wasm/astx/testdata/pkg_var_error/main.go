package main

type PageConfig struct{ ClientSideState func() }

var counter int

func myFunc() { counter++ }

var Page = PageConfig{ClientSideState: myFunc}

func main() {}
