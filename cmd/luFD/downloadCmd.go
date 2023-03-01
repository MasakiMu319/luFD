package main

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"luFD/internal/errorHandle"
	"luFD/internal/executioner"
	"luFD/internal/tool"
	"os"
)

var (
	conc   int
	baidu  bool
	output string
)

func init() {
	rootCmd.AddCommand(downloadComd)
	downloadComd.Flags().IntVarP(&conc, "goroutines count", "c", 1, "default is your CPU threads count")
	downloadComd.Flags().BoolVarP(&baidu, "baidu", "b", false, "download from baidu")
	downloadComd.Flags().StringVarP(&output, "output", "o", "", "output file name")

}

var downloadComd = &cobra.Command{
	Use:     "download",
	Short:   "download files from url",
	Example: `luFD download URL`,
	Run: func(cmd *cobra.Command, args []string) {
		// args[0] is the url, parse value is not in args
		errorHandle.ExitWithError(download(args))
	},
}

func download(args []string) error {
	folderFrom, err := tool.GetFolderFrom(args[0])
	if err != nil {
		return errors.WithStack(err)
	}
	if tool.IsFolderExisted(folderFrom) {
		fmt.Printf("Task already exist, remove it first \n")
		from, err := tool.GetFolderFrom(args[0])
		if err != nil {
			return errors.WithStack(err)
		}
		if err := os.RemoveAll(from); err != nil {
			return errors.WithStack(err)
		}
	}
	return executioner.Do(args[0], nil, conc, baidu)
}
