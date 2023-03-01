package downloader

import (
	"crypto/tls"
	"fmt"
	"github.com/cheggaaa/pb"
	"github.com/pkg/errors"
	"io"
	"luFD/internal/tool"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	acceptRange   = "Accept-Ranges"
	contentLength = "Content-Length"
	netDisk       = "pan.baidu.com"
)

type HTTPDownloader struct {
	URL            string               // download url
	File           string               // file name
	Part           int64                // number of parts
	Len            int64                // file length
	SkipTLS        bool                 // skip tls verify
	DownloadRanges []tool.DownloadRange // download range
	Resume         bool                 // resume download
}

type userAgentTransport struct {
	userAgent string            // user agent
	rt        http.RoundTripper // http round tripper
}

var (
	client = &http.Client{
		Transport: tr,
	}
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		// disable keep-alive connections, this is to avoid the problem of
		// Unsolicited response received on idle HTTP channel starting with "HTTP/1.1 100 Continue"
		// TODO: find a better way to solve this problem
		DisableKeepAlives: true,
	}
	skipTLS = true
)

func (uat *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = cloneRequest(req)
	req.Header.Set("User-Agent", uat.userAgent)
	return uat.rt.RoundTrip(req)
}

func cloneRequest(req *http.Request) *http.Request {
	r2 := new(http.Request)
	*r2 = *req
	r2.Header = make(http.Header, len(req.Header))
	for k, s := range req.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}

func NewHTTPDownloader(inputUrl string, par int, baidu bool) (*HTTPDownloader, error) {
	// resume value is true by default
	resume := true

	req, err := http.NewRequest("GET", inputUrl, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if baidu {
		// set user agent for baidu netDisk
		client.Transport = &userAgentTransport{
			userAgent: netDisk,
			rt:        tr,
		}
	} else {
		client.Transport = tr
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// check if the server supports resume download
	if resp.Header.Get(acceptRange) != "bytes" {
		resume = false
		par = 1
	}

	// clen is the content length of the file
	clen := resp.Header.Get(contentLength)
	if clen == "" {
		fmt.Printf("Content-Length is not set, using 1 part\n")
		clen = "1"
		par = 1
		resume = false
	}

	fmt.Printf("Start downloading with %d parts\n", par)

	// parse content length to int64, because the content length is string
	length, err := strconv.ParseInt(clen, 10, 64)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// calculate the size of the file, content length is in bytes
	sizeInMb := float64(length) / 1024 / 1024
	if clen == "1" {
		fmt.Printf("Download size: not specified\n")
	} else if sizeInMb < 1024 {
		fmt.Printf("Download size: %.2f MB\n", sizeInMb)
	} else {
		fmt.Printf("Download size: %.2f GB\n", sizeInMb/1024)
	}

	// get the file name from the url
	file := filepath.Base(inputUrl)
	if len(file) >= 15 {
		// if the file name is too long, use the last 15 characters,
		// this is because the file type is always at the end of the file name
		file = file[len(file)-15:]
	}

	dl := &HTTPDownloader{
		URL:     inputUrl,
		File:    file,
		Part:    int64(par),
		Len:     length,
		SkipTLS: skipTLS,
		Resume:  resume,
	}
	// calculate the download range
	dl.DownloadRanges, err = partCalculator(int64(par), length, inputUrl)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return dl, nil
}

func partCalculator(part int64, length int64, url string) ([]tool.DownloadRange, error) {
	dlRange := []tool.DownloadRange{}

	uSize := length / part
	for i := int64(0); i < part; i++ {
		from := uSize * i
		to := uSize * (i + 1)
		if i < part-1 {
			to = uSize*(i+1) - 1
		} else {
			// TODO: check if this is correct
			to = length
		}

		file := filepath.Base(url)
		if len(file) >= 15 {
			file = file[:15]
		}

		// get the folder name of the part file
		folder, err := tool.GetFolderFrom(url)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if err := tool.Mkdir(folder); err != nil {
			return nil, errors.WithStack(err)
		}

		fName := fmt.Sprintf("%s.part%d", file, i)
		fPath := filepath.Join(folder, fName)
		dlRange = append(dlRange, tool.DownloadRange{
			URL:       url,
			Path:      fPath,
			RangeFrom: from,
			RangeTo:   to,
		})
	}
	return dlRange, nil
}

func (d *HTTPDownloader) Downloading(doneChan chan bool, fileChan chan string,
	errorChan chan error, interrupt chan bool, stateSaveChan chan tool.DownloadRange) {
	// TODO: update pb to v3, maybe some bugs
	var bars []*pb.ProgressBar
	var barPool *pb.Pool
	var err error

	if tool.DisappearProgressBar() {
		bars = []*pb.ProgressBar{}
		for i, part := range d.DownloadRanges {
			// TODO: update pb to v3 or use progressbar
			// newBar need to set refresh rate more than 1s, otherwise it maybe get EOF error,
			// maybe a bug of pb, or the target file is too large, this will happen when the
			// file is larger than 1GB
			newBar := pb.New64(part.RangeTo - part.RangeFrom).SetUnits(pb.U_BYTES).SetRefreshRate(time.Second).Prefix(fmt.Sprintf("%s - %d", d.File, i))
			newBar.ShowBar = true
			bars = append(bars, newBar)
		}
		barPool, err = pb.StartPool(bars...)
		if err != nil {
			errorChan <- errors.WithStack(err)
			return
		}

		ws := new(sync.WaitGroup)
		for i, p := range d.DownloadRanges {
			ws.Add(1)
			go func(d *HTTPDownloader, loop int64, part tool.DownloadRange) {
				defer ws.Done()
				bar := new(pb.ProgressBar)

				if tool.DisappearProgressBar() {
					bar = bars[loop]
				}

				ranges := ""
				if part.RangeTo != d.Len {
					ranges = fmt.Sprintf("bytes=%d-%d", part.RangeFrom, part.RangeTo)
				} else {
					ranges = fmt.Sprintf("bytes=%d-", part.RangeFrom)
				}

				req, err := http.NewRequest("GET", d.URL, nil)
				if err != nil {
					errorChan <- errors.WithStack(err)
					return
				}

				if d.Part > 1 {
					req.Header.Set("Range", ranges)
					if err != nil {
						errorChan <- errors.WithStack(err)
						return
					}
				}

				resp, err := client.Do(req)
				if err != nil {
					errorChan <- errors.WithStack(err)
					return
				}
				defer resp.Body.Close()

				f, err := os.OpenFile(part.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
				if err != nil {
					errorChan <- errors.WithStack(err)
					return
				}

				defer func(f *os.File) {
					err := f.Close()
					if err != nil {
						errorChan <- errors.WithStack(err)
						return
					}
				}(f)

				var writer io.Writer
				if tool.DisappearProgressBar() {
					// writer is an io.MultiWriter, it will write to the file and the progress bar
					writer = io.MultiWriter(f, bar)
				} else {
					writer = io.MultiWriter(f)
				}

				// current is the current written bytes
				current := int64(0)
				for {
					select {
					case <-interrupt:
						stateSaveChan <- tool.DownloadRange{
							URL:       d.URL,
							Path:      part.Path,
							RangeFrom: part.RangeFrom + current,
							RangeTo:   part.RangeTo,
						}
						return
					default:
						// io.CopyN will copy the data from the resp.Body to the writer,
						// this for loop will run until the resp.Body is EOF, EOF means
						// this file or part is downloaded
						written, err := io.CopyN(writer, resp.Body, 1024)
						current += written
						if err != nil {
							if err != io.EOF {
								errorChan <- errors.WithStack(err)
								return
							}
							fileChan <- part.Path
							return
						}
					}
				}
			}(d, int64(i), p)
		}
		ws.Wait()

		err = barPool.Stop()
		if err != nil {
			errorChan <- errors.WithStack(err)
			return
		}
	}
	doneChan <- true
}
