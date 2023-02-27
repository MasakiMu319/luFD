package main

import (
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   os.Args[0],
	Short: "file downloader written in Go",
}
