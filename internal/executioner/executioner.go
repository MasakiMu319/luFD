package executioner

import (
	"fmt"
	"github.com/pkg/errors"
	"luFD/internal/downloader"
	"luFD/internal/merger"
	"luFD/internal/tool"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func Do(url string, state *tool.State, conc int) error {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	files := []string{}
	parts := []tool.DownloadRange{}
	isInterrupted := false
	doneChan := make(chan bool, conc)
	fileChan := make(chan string, conc)
	errorChan := make(chan error, 1)
	stateChan := make(chan tool.DownloadRange, 1)
	interruptChan := make(chan bool, conc)

	var dl *downloader.HTTPDownloader
	var err error

	if state == nil {
		dl, err = downloader.NewHTTPDownloader(url, conc)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		dl = &downloader.HTTPDownloader{
			URL:            state.URL,
			File:           filepath.Base(state.URL),
			Part:           int64(len(state.DownloadRanges)),
			SkipTLS:        true,
			DownloadRanges: state.DownloadRanges,
			Resume:         true,
		}
	}
	go dl.Downloading(doneChan, fileChan, errorChan, interruptChan, stateChan)
	for {
		select {
		case <-signalChan:
			isInterrupted = true
			for conc > 0 {
				interruptChan <- true
				conc--
			}
		case file := <-fileChan:
			files = append(files, file)
		case err = <-errorChan:
			return errors.WithStack(err)
		case part := <-stateChan:
			parts = append(parts, part)
		case <-doneChan:
			if isInterrupted {
				if dl.Resume {
					fmt.Printf("Download interrupted, saving state...\n")
					s := &tool.State{
						URL:            dl.URL,
						DownloadRanges: parts,
					}
					if err = s.Save(); err != nil {
						return errors.WithStack(err)
					}
					return nil
				} else {
					fmt.Printf("Download interrupted, but not resumable\n")
					return nil
				}
			} else {
				err = merger.MergeFiles(files, filepath.Base(url))
				if err != nil {
					return errors.WithStack(err)
				}
				folder, err := tool.GetFolderFrom(url)
				if err != nil {
					return errors.WithStack(err)
				}
				err = os.RemoveAll(folder)
				if err != nil {
					return errors.WithStack(err)
				}
				return nil
			}
		}
	}
}
