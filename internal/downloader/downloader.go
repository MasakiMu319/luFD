package downloader

import (
	"crypto/tls"
	"fmt"
	"github.com/cheggaaa/pb"
	"github.com/pkg/errors"
	"io"
	"luFD/internal/tool"
	"net"
	"net/http"
	stdurl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	acceptRange   = "Accept-Ranges"
	contentLength = "Content-Length"
)

var (
	client = &http.Client{
		Transport: tr,
	}
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	skipTLS = true
)

type HTTPDownloader struct {
	URL            string
	File           string
	Part           int64
	Len            int64
	IPs            []string
	SkipTLS        bool
	DownloadRanges []tool.DownloadRange
	Resume         bool
}

func NewHTTPDownloader(url string, par int) (*HTTPDownloader, error) {
	resume := true

	parse, err := stdurl.Parse(url)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ips, err := net.LookupIP(parse.Host)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ipStr := tool.GetIPv4(ips)
	fmt.Printf("Download IP is: %s\n", strings.Join(ipStr, " | "))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if resp.Header.Get(acceptRange) != "bytes" {
		resume = false
		par = 1
	}

	clen := resp.Header.Get(contentLength)
	if clen == "" {
		fmt.Printf("Content-Length is not set, using 1 part\n")
		clen = "1"
		par = 1
		resume = false
	}

	fmt.Printf("Start downloading with %d parts\n", par)

	len, err := strconv.ParseInt(clen, 10, 64)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	sizeInMb := float64(len) / 1024 / 1024
	if clen == "1" {
		fmt.Printf("Download size: not specified\n")
	} else if sizeInMb < 1024 {
		fmt.Printf("Download size: %.2f MB\n", sizeInMb)
	} else {
		fmt.Printf("Download size: %.2f GB\n", sizeInMb/1024)
	}

	file := filepath.Base(url)
	ret := new(HTTPDownloader)
	ret.URL = url
	ret.File = file
	ret.Part = int64(par)
	ret.Len = len
	ret.IPs = ipStr
	ret.SkipTLS = skipTLS
	ret.DownloadRanges, err = partCalculator(int64(par), len, url)
	ret.Resume = resume

	return ret, nil
}

func partCalculator(part int64, len int64, url string) ([]tool.DownloadRange, error) {
	ret := []tool.DownloadRange{}

	for i := int64(0); i < part; i++ {
		from := (len / part) * i
		to := (len / part) * (i + 1)
		if i < part-1 {
			to = (len/part)*(i+1) - 1
		} else {
			to = len
		}

		file := filepath.Base(url)
		folder, err := tool.GetFolderFrom(url)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if err := tool.Mkdir(folder); err != nil {
			return nil, errors.WithStack(err)
		}

		fname := fmt.Sprintf("%s.part%d", file, i)
		fpath := filepath.Join(folder, fname)
		ret = append(ret, tool.DownloadRange{
			URL:       url,
			Path:      fpath,
			RangeFrom: from,
			RangeTo:   to,
		})
	}
	return ret, nil
}

func (d *HTTPDownloader) Downloading(doneChan chan bool, fileChan chan string,
	errorChan chan error, interrupt chan bool, stateSaveChan chan tool.DownloadRange) {
	// TODO: update pb to v3, maybe some bugs
	var bars []*pb.ProgressBar
	var barpool *pb.Pool
	var err error

	if tool.DisappearProgressBar() {
		bars = []*pb.ProgressBar{}
		for i, part := range d.DownloadRanges {
			newBar := pb.New64(part.RangeTo - part.RangeFrom).SetUnits(pb.U_BYTES).Prefix(fmt.Sprintf("%s - %d", d.File, i))
			newBar.ShowBar = true
			bars = append(bars, newBar)
		}
		barpool, err = pb.StartPool(bars...)
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

				var writer io.Writer
				if tool.DisappearProgressBar() {
					writer = io.MultiWriter(f, bar)
				} else {
					writer = io.MultiWriter(f)
				}

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
						written, err := io.CopyN(writer, resp.Body, 100)
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
				err = f.Close()
				if err != nil {
					errorChan <- errors.WithStack(err)
					return
				}
			}(d, int64(i), p)
		}
		ws.Wait()

		err = barpool.Stop()
		if err != nil {
			errorChan <- errors.WithStack(err)
			return
		}
	}
	doneChan <- true
}
