package main

type PageConfig struct{ ClientSideState func() }

func helper1() { helper2() }
func helper2() { helper3() }
func helper3() {}

var Page = PageConfig{ClientSideState: helper1}

func main() {}
