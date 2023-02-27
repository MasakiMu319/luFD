package main

import "log"

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln("Something went wrong while executing the command.")
		return
	}
}
