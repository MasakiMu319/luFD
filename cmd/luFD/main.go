package main

import (
	"log"
	"runtime"
)

func main() {
	runtime.GOMAXPROCS(8)
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln("Something went wrong while executing the command.")
		return
	}
}
