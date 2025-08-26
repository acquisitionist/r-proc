/*
MIT License

Copyright (c) 2025 The R-Proc Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"
	"github.com/klauspost/compress/zstd"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/semaphore"
)

var ErrProcessClosed = errors.New("process: Processor closed")

type Processor struct {
	Threads int
	Input   string
	Output  string

	Field       string
	Values      []string
	ValuesRegex []*regexp.Regexp
	FileFilter  *regexp.Regexp
	MatchMode   string

	ErrorLog   *slog.Logger
	inShutdown atomic.Bool

	mu         sync.Mutex
	onShutdown []func()
	wg         sync.WaitGroup
}

func (p *Processor) shuttingDown() bool {
	return p.inShutdown.Load()
}

func (p *Processor) Shutdown(ctx context.Context) error {
	p.inShutdown.Store(true)

	p.mu.Lock()
	for _, f := range p.onShutdown {
		go f()
	}
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (p *Processor) ProcessAndServe() error {
	if p.shuttingDown() {
		return ErrProcessClosed
	}

	if p.MatchMode == "regex" {
		for _, value := range p.Values {
			p.ValuesRegex = append(p.ValuesRegex, regexp.MustCompile(value))
		}
	}

	var f []string
	err := filepath.Walk(p.Input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(info.Name()) != ".zst" {
			return nil
		}

		if !p.FileFilter.MatchString(info.Name()) {
			return nil
		}

		f = append(f, path)
		p.ErrorLog.Info("found input file", "path", path)
		return nil
	})

	if err != nil {
		return err
	}

	if len(f) == 0 {
		p.ErrorLog.Warn("no input files found in input folder", "input", p.Input)
		return nil
	}
	return p.Serve(f)
}

type contextKey struct {
	name string
}

var ServerContextKey = &contextKey{"process-server"}

func (p *Processor) Serve(f []string) error {
	sem := semaphore.NewWeighted(int64(p.Threads))
	baseCtx := context.Background()
	ctx := context.WithValue(baseCtx, ServerContextKey, p)

	var zstdOpts = []zstd.DOption{
		zstd.WithDecoderMaxWindow(1 << 32),
		zstd.WithDecoderMaxMemory(1 << 33),
		zstd.WithDecoderLowmem(false),
		zstd.WithDecoderConcurrency(0),
	}

	barz := mpb.New(mpb.WithWidth(64))

	for _, file := range f {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}

		p.wg.Go(func() {
			defer func() {
				sem.Release(1)
				if pv := recover(); pv != nil {
					p.ErrorLog.Error("panic recovered in worker", "panic", pv)
				}
			}()

			info, err := os.Stat(file)
			if err != nil {
				p.ErrorLog.Error("failed to get file information", "path", file, "err", err)
				panic(err)
			}
			totalBytes := info.Size()

			input, err := os.Open(file)
			if err != nil {
				p.ErrorLog.Error("failed to open file", "path", file, "err", err)
				panic(err)
			}
			defer input.Close()

			zstdReader, err := zstd.NewReader(input, zstdOpts...)
			if err != nil {
				p.ErrorLog.Error("failed to create zstd reader", "path", file, "err", err)
				panic(err)
			}
			defer zstdReader.Close()

			scanner := bufio.NewScanner(zstdReader)
			scanner.Buffer(make([]byte, 64<<10), 512<<20)

			bar := barz.New(totalBytes,
				mpb.BarStyle().Lbound("╢").Filler("▌").Tip("▌").Padding("░").Rbound("╟"),
				mpb.PrependDecorators(
					decor.Name(filepath.Base(file)+":", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
					decor.Counters(decor.SizeB1024(0), "% .2f / % .2f", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
				),
				mpb.AppendDecorators(
					decor.Percentage(decor.WCSyncWidth, decor.WC{C: decor.DindentRight | decor.DextraSpace}),
					decor.Name("Avg. ETA:", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
					decor.OnComplete(
						decor.AverageETA(decor.ET_STYLE_GO, decor.WC{C: decor.DindentRight | decor.DextraSpace}),
						"done",
					),
				),
			)

			for scanner.Scan() {
				if p.shuttingDown() {
					p.ErrorLog.WarnContext(ctx,
						"skipping further processing of file",
						"path", file,
					)
					return
				}

				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}

				fieldVal := jsoniter.Get(line, p.Field).ToString()
				if fieldVal == "" {
					continue
				}

				matched := false
				for i, val := range p.Values {
					switch p.MatchMode {
					case "regex":
						re := p.ValuesRegex[i]
						matched = re.MatchString(fieldVal)
					case "partial":
						matched = strings.Contains(strings.ToLower(fieldVal), strings.ToLower(val))
					case "exact":
						matched = strings.EqualFold(fieldVal, val)
					}
					if matched {
						p.write(file, val, string(line))
						break
					}
				}
				bar.IncrBy(512)
			}
		})

	}

	p.wg.Wait()
	if p.shuttingDown() {
		return ErrProcessClosed
	}

	return nil
}

func (p *Processor) write(inputPath, value, line string) {
	outFileName := filepath.Join(p.Output, fmt.Sprintf("%s_%s.ndjson", strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath)), value))

	outFile, err := os.OpenFile(outFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		p.ErrorLog.Warn("failed to open output file",
			"path", outFileName,
			"err", err,
		)
		return
	}
	defer outFile.Close()

	if _, err := outFile.WriteString(line + "\n"); err != nil {
		p.ErrorLog.Warn("failed to write to output file",
			"path", outFileName,
			"err", err,
		)
		return
	}
}
