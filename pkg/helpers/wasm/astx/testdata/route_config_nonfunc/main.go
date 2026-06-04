package main

import "example.com/helpers/helpers/routes"

var notAFunc = 42

var Page = routes.RouteConfig{
	ClientSideState: notAFunc,
}

func main() {}
