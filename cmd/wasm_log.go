package cmd

import (
	"fmt"
	"time"
)

const (
	ansiReset       = "\033[0m"
	ansiBold        = "\033[1m"
	ansiWhite       = "\033[37m"
	ansiRed         = "\033[31m"
	ansiPurpleLight = "\033[38;5;183m"
)

const wasmTag = ansiBold + ansiPurpleLight + "WASM" + ansiReset

func wasmTimestamp() string {
	return ansiWhite + time.Now().Format("2006/01/02 15:04:05") + ansiReset
}

func wasmLogf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiWhite+format+ansiReset+"\n", args...)
}

func wasmErrorf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiRed+format+ansiReset+"\n", args...)
}
