package main

import (
	"runtime"
)

func main() {
	// ゴルーチンの最大数を設定しておく
	runtime.GOMAXPROCS(runtime.NumCPU())
}