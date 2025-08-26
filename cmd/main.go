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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/lmittmann/tint"
	"gopkg.in/ini.v1"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))
	defer func() {
		if r := recover(); r != nil {
			logger.Error(
				"panic recovered",
				"error", fmt.Sprintf("%v", r),
				"trace", string(debug.Stack()),
			)
			os.Exit(1)
		}
	}()
	if err := run(logger); err != nil {
		logger.Error(err.Error(), "trace", string(debug.Stack()))
		os.Exit(1)
	}
}

type config struct {
	Threads int `ini:"threads" validate:"required,gte=1"`

	Paths struct {
		Config string `validate:"required,file"`
		Input  string `ini:"input" validate:"required,dir"`
		Output string `ini:"output" validate:"required,dir"`
	} `ini:"paths"`

	Filter struct {
		Field      string   `ini:"field" validate:"required,oneof=subreddit author title selftext body domain"`
		Values     []string `ini:"values" validate:"required,dive,required"`
		FileFilter string   `ini:"file_filter" validate:"required"`
		MatchMode  string   `ini:"match_mode" validate:"required,oneof= exact partial regex"`
	} `ini:"filters"`
}

type application struct {
	config config
	logger *slog.Logger
	wg     sync.WaitGroup
}

func run(logger *slog.Logger) error {
	var cfg config

	flag.StringVar(&cfg.Paths.Config, "config", "config.ini", "Configuration file path")
	flag.Parse()

	v := validator.New(validator.WithRequiredStructEnabled())
	ini, iniErr := ini.Load(cfg.Paths.Config)
	if iniErr != nil {
		return iniErr
	}
	mapErr := ini.MapTo(&cfg)
	if mapErr != nil {
		return mapErr
	}
	if cfgErr := v.Struct(cfg); cfgErr != nil {
		return cfgErr
	}
	app := application{config: cfg, logger: logger}
	return app.serveProcessor()
}
