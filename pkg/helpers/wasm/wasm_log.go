package helpers

import (
	"fmt"
	"time"
)

const (
	ansiReset      = "\033[0m"
	ansiBold       = "\033[1m"
	ansiWhite       = "\033[37m"
	ansiCyan        = "\033[36m"           // name — matches GET/method cyan
	ansiRed         = "\033[31m"
	ansiYellow      = "\033[33m"           // compression method — matches size yellow in existing logs
	ansiLightGreen  = "\033[38;5;120m"     // final size — matches status code green
	ansiBlue        = "\033[38;5;75m"      // raw size — matches bytes blue in existing logs
	ansiPurpleLight = "\033[38;5;183m"     // WASM tag — lighter purple to match GET cyan brightness
	ansiGray        = "\033[38;5;244m"     // dim gray — arrows
)

const wasmTag = ansiBold + ansiPurpleLight + "WASM" + ansiReset

func wasmTimestamp() string {
	return ansiWhite + time.Now().Format("2006/01/02 15:04:05") + ansiReset
}

func wasmUpToDate(name string) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiCyan+"%s"+ansiReset+ansiGray+" → "+ansiReset+ansiLightGreen+"up to date"+ansiReset+"\n", name)
}

func wasmLogf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiCyan+format+ansiReset+"\n", args...)
}

func wasmErrorf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiRed+format+ansiReset+"\n", args...)
}

func wasmWarnf(format string, args ...any) {
	fmt.Printf(wasmTimestamp()+" "+wasmTag+" "+ansiYellow+format+ansiReset+"\n", args...)
}

// wasmBuildResult prints the coloured build-result line:
//
//	2006/01/02 15:04:05 WASM <name> → <rawSize> → <finalSize> (<compression>)
//
// name: white  raw size: blue  final size: light green  compression: yellow
func wasmBuildResult(name, rawSize, finalSize, compression string) {
	fmt.Printf(
		wasmTimestamp()+" "+wasmTag+" "+
			ansiCyan+"%s"+ansiReset+
			ansiGray+" → "+ansiReset+
			ansiBlue+"%s"+ansiReset+
			ansiGray+" → "+ansiReset+
			ansiLightGreen+"%s"+ansiReset+
			" "+ansiYellow+"(%s)"+ansiReset+
			"\n",
		name, rawSize, finalSize, compression,
	)
}
