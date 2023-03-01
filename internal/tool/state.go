package tool

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

var (
	SaveFolder = "Downloads/luFDTemp"
)

type State struct {
	URL            string          // URL is the url of the file to download
	DownloadRanges []DownloadRange // DownloadRanges is a list of DownloadRange, used to save the state of downloading
}

type DownloadRange struct {
	URL       string // URL is the url of the file to download
	Path      string // Path is the path of the download file to save
	RangeFrom int64  // RangeFrom is the start of the range to download
	RangeTo   int64  // RangeTo is the end of the range to download
}

func (state *State) Save() error {
	folder, err := GetFolderFrom(state.URL)
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf("Saving states data in %s\n", folder)
	err = Mkdir(folder)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, part := range state.DownloadRanges {
		err = os.Rename(part.Path, filepath.Join(folder, filepath.Base(part.Path)))
		if err != nil {
			return errors.WithStack(err)
		}
	}

	y, err := yaml.Marshal(state)
	if err != nil {
		return errors.WithStack(err)
	}
	return os.WriteFile(filepath.Join(folder, "state.yaml"), y, 0644)
}
