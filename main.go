package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/goyek/goyek"
	"github.com/jaschaephraim/lrserver"
	"github.com/otiai10/copy"
	"github.com/radovskyb/watcher"
)

var triggerReload = make(chan struct{})

type options struct {
	ESBuild api.BuildOptions
	Watch   struct {
		Path    string
		Exclude []string
	}
	Copy []struct {
		Src  string
		Dest string
	}
	Replace []struct {
		Pattern string
		Search  string
		Replace string
	}
}

func main() {
	flow := &goyek.Flow{}
	opts := options{}

	cfgPathParam := flow.RegisterStringParam(goyek.StringParam{
		Name:    "c",
		Usage:   "Path to config file config file.",
		Default: "./.gowebbuild.json",
	})

	prodParam := flow.RegisterBoolParam(goyek.BoolParam{
		Name:    "p",
		Usage:   "Use production ready build settings",
		Default: false,
	})

	buildOnly := goyek.Task{
		Name:   "build",
		Usage:  "",
		Params: goyek.Params{cfgPathParam, prodParam},
		Action: func(tf *goyek.TF) {
			cfgPath := cfgPathParam.Get(tf)
			cfgContent, err := os.ReadFile(cfgPath)

			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			err = json.Unmarshal(cfgContent, &opts)
			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			cp(opts)
			replace(opts)

			if prodParam.Get(tf) {
				opts.ESBuild.MinifyIdentifiers = true
				opts.ESBuild.MinifySyntax = true
				opts.ESBuild.MinifyWhitespace = true
				opts.ESBuild.Sourcemap = api.SourceMapNone
			}

			api.Build(opts.ESBuild)
		},
	}

	watchFrontend := goyek.Task{
		Name:   "watch-frontend",
		Usage:  "",
		Params: goyek.Params{cfgPathParam},
		Action: func(tf *goyek.TF) {
			cfgPath := cfgPathParam.Get(tf)
			cfgContent, err := os.ReadFile(cfgPath)

			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			err = json.Unmarshal(cfgContent, &opts)
			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)

			fmt.Println("Starting live reload server")

			go func() {
				w := watcher.New()
				w.SetMaxEvents(1)
				w.FilterOps(watcher.Write, watcher.Rename, watcher.Move, watcher.Create, watcher.Remove)

				if len(opts.Watch.Exclude) > 0 {
					w.Ignore(opts.Watch.Exclude...)
				}

				if err := w.AddRecursive(opts.Watch.Path); err != nil {
					fmt.Println(err.Error())
					os.Exit(1)
				}

				go func() {
					for {
						select {
						case event := <-w.Event:
							fmt.Printf("File %s changed\n", event.Name())
							cp(opts)
							build(opts)
							replace(opts)
						case err := <-w.Error:
							fmt.Println(err.Error())
						case <-w.Closed:
							return
						}
					}
				}()

				fmt.Printf("Watching %d elements in %s\n", len(w.WatchedFiles()), opts.Watch.Path)

				cp(opts)
				build(opts)
				replace(opts)

				if err := w.Start(time.Millisecond * 100); err != nil {
					fmt.Println(err.Error())
				}
			}()

			go func() {
				lr := lrserver.New(lrserver.DefaultName, lrserver.DefaultPort)

				go func() {
					for {
						<-triggerReload
						lr.Reload("")
					}
				}()

				lr.SetStatusLog(nil)
				err := lr.ListenAndServe()
				if err != nil {
					panic(err)
				}
			}()

			<-c
			fmt.Println("\nExit")
			os.Exit(0)
		},
	}

	flow.DefaultTask = flow.Register(watchFrontend)
	flow.Register(buildOnly)
	flow.Main()
}

func cp(opts options) {
	if len(opts.Copy) == 0 {
		fmt.Println("Nothing to copy")
		return
	}
	for _, op := range opts.Copy {
		paths, err := filepath.Glob(op.Src)
		if err != nil {
			fmt.Printf("Invalid glob pattern: %s\n", op.Src)
			continue
		}

		destIsDir := isDir(op.Dest)
		for _, p := range paths {
			d := op.Dest

			if destIsDir && isFile(p) {
				d = filepath.Join(d, filepath.Base(p))
			}
			err := copy.Copy(p, d)
			fmt.Printf("Copying %s to %s\n", p, d)

			if err != nil {
				fmt.Printf("Failed to copy %s: %v\n", p, err)
				continue
			}
		}
	}
}

func replace(opts options) {
	if len(opts.Replace) == 0 {
		fmt.Println("Nothing to replace")
		return
	}
	for _, op := range opts.Replace {
		paths, err := filepath.Glob(op.Pattern)
		if err != nil {
			fmt.Printf("Invalid glob pattern: %s\n", op.Pattern)
			continue
		}
		fmt.Printf("Paths: %+v\n", paths)
		for _, p := range paths {
			if !isFile(p) {
				continue
			}

			read, err := ioutil.ReadFile(p)
			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			r := op.Replace
			if strings.HasPrefix(op.Replace, "$") {
				r = os.ExpandEnv(op.Replace)
			}

			if strings.Contains(string(read), op.Search) {
				fmt.Printf("Replacing '%s' with '%s' in %s\n", op.Search, r, p)
				newContents := strings.Replace(string(read), op.Search, r, -1)
				err = ioutil.WriteFile(p, []byte(newContents), 0)
				if err != nil {
					fmt.Printf("%+v\n", err)
					os.Exit(1)
				}
			}
		}
	}
}

func isFile(path string) bool {
	stat, err := os.Stat(path)

	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	return !stat.IsDir()
}

func isDir(path string) bool {
	stat, err := os.Stat(path)

	if errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(path, 0755)
		return true
	}

	return err == nil && stat.IsDir()
}

func build(opts options) {
	result := api.Build(opts.ESBuild)

	if len(result.Errors) == 0 {
		triggerReload <- struct{}{}
	}
}
