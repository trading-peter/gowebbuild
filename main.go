package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
		Path string
	}
	Copy []struct {
		Src  string
		Dest string
	}
}

func main() {
	opts := options{}
	cfgContent, err := os.ReadFile("./.gowebbuild.json")

	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(cfgContent, &opts)
	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	flow := &goyek.Flow{}

	flow.Register(goyek.Task{
		Name:  "watch-frontend",
		Usage: "",
		Action: func(tf *goyek.TF) {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)

			fmt.Println("Starting live reload server")

			go func() {
				w := watcher.New()
				w.SetMaxEvents(1)
				w.FilterOps(watcher.Write, watcher.Rename, watcher.Move, watcher.Create, watcher.Remove)
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
						case err := <-w.Error:
							fmt.Println(err.Error())
						case <-w.Closed:
							return
						}
					}
				}()

				fmt.Printf("Watching %d elements in %s\n", len(w.WatchedFiles()), opts.Watch.Path)

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
	})

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
