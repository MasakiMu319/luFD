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

func Do(url string, state *tool.State, conc int, baidu bool) error {
	signalChan := make(chan os.Signal, 1)
	// signal.Notify will sign up for the notification of the specified signals
	signal.Notify(signalChan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	files := []string{}
	parts := []tool.DownloadRange{}
	// isInterrupted is used to indicate whether the download is interrupted,
	// if it is interrupted, the download will be resumed, and saved to the state file
	isInterrupted := false
	// doneChan is used to indicate that the download is complete
	doneChan := make(chan bool, conc)
	// fileChan is used to receive the file name of the downloaded file
	// TODO: later back to check
	fileChan := make(chan string, conc)
	errorChan := make(chan error, 1)
	stateChan := make(chan tool.DownloadRange, 1)
	// TODO: why is it necessary have a interruptChan? SignalChan is enough?
	interruptChan := make(chan bool, conc)

	var dl *downloader.HTTPDownloader
	var err error

	// for the first time download, state is always nil
	if state == nil {
		dl, err = downloader.NewHTTPDownloader(url, conc, baidu)
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
				baseUrl := filepath.Base(url)
				if len(baseUrl) > 15 {
					baseUrl = baseUrl[len(baseUrl)-15:]
				}
				err = merger.MergeFiles(files, baseUrl)
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
