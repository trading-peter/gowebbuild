package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jaschaephraim/lrserver"
	"github.com/radovskyb/watcher"
	"github.com/urfave/cli/v2"
)

func watchAction(ctx *cli.Context) error {
	cfgPath := ctx.String("c")
	os.Chdir(filepath.Dir(cfgPath))
	optsSetups := readCfg(cfgPath)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	for i := range optsSetups {
		opts := optsSetups[i]

		go func(opts options) {
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
		}(opts)

		if opts.Serve.Path != "" {
			go func() {
				port := 8888
				if opts.Serve.Port != 0 {
					port = opts.Serve.Port
				}

				http.Handle("/", http.FileServer(http.Dir(opts.Serve.Path)))

				fmt.Printf("Serving contents of %s at :%d\n", opts.Serve.Path, port)
				err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)

				if err != nil {
					fmt.Printf("%+v\n", err.Error())
					os.Exit(1)
				}
			}()
		}

		if opts.Link.From != "" {
			reqBuildCh := link(opts.Link.From, opts.Link.To)

			go func() {
				for range reqBuildCh {
					cp(opts)
					build(opts)
					replace(opts)
				}
			}()
		}
	}

	go func() {
		fmt.Println("Starting live reload server")
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
	fmt.Println("\nStopped watching")

	return nil
}
