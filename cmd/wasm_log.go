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
	ansiCyan        = "\033[36m"
	ansiLightGreen  = "\033[38;5;120m"
)

const wasmTag = ansiBold + ansiPurpleLight + "WASM" + ansiReset

func wasmTimestamp() string {
	return ansiWhite + time.Now().Format("2006/01/02 15:04:05") + ansiReset
}

func wasmLogf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiCyan+format+ansiReset+"\n", args...)
}

// wasmCount formats a count+label pair: number in light green, label in cyan.
// Ends in cyan so surrounding text in wasmLogf stays cyan after substitution.
func wasmCount(n int, label string) string {
	return fmt.Sprintf(ansiLightGreen+"%d"+ansiCyan+" %s", n, label)
}

func wasmErrorf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiRed+format+ansiReset+"\n", args...)
}
