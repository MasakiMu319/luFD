package main

import (
	"github.com/spf13/cobra"
	"time"
)

var (
	Version string
	Commit  string
	Date    string
)

func init() {
	rootCmd.AddCommand(versionCmd)
	Version = "test:latest"
	Commit = "for test"
	// about date
	Date = time.Now().Format("2006-01-02")
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "prints meta info",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("Version: ", Version)
		cmd.Println("Commit: ", Commit)
		cmd.Println("Date: ", Date)
	},
}
