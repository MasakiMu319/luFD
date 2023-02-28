package tool

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

var (
	SaveFolder = "Downloads/luFD"
)

type State struct {
	URL            string
	DownloadRanges []DownloadRange
}

type DownloadRange struct {
	URL       string
	Path      string
	RangeFrom int64
	RangeTo   int64
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