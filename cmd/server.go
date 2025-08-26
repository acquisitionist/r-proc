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
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

const (
	defaultShutdownPeriod = 30 * time.Second
)

func (app *application) serveProcessor() error {
	srv := &Processor{
		Input:      app.config.Paths.Input,
		Output:     app.config.Paths.Output,
		Threads:    app.config.Threads,
		Field:      app.config.Filter.Field,
		Values:     app.config.Filter.Values,
		FileFilter: regexp.MustCompile(app.config.Filter.FileFilter),
		MatchMode:  app.config.Filter.MatchMode,

		ErrorLog: slog.New(app.logger.Handler()),
	}

	err := app.serve(srv)
	if err != nil {
		return err
	}

	app.wg.Wait()
	return nil
}

func (app *application) serve(srv *Processor) error {
	shutdownErrorChan := make(chan error)

	go func() {
		quitChan := make(chan os.Signal, 1)
		signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(quitChan)

		<-quitChan

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()

		shutdownErrorChan <- srv.Shutdown(ctx)
	}()

	app.logger.Info("starting processor", slog.Group("processor"))

	err := srv.ProcessAndServe()
	if !errors.Is(err, ErrProcessClosed) {
		return err
	}

	if err := <-shutdownErrorChan; err != nil {
		return err
	}

	app.logger.Info("stopped processor", slog.Group("processor"))
	return nil
}
