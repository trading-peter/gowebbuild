package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jaschaephraim/lrserver"
	"github.com/radovskyb/watcher"
	"github.com/urfave/cli/v2"
)

func watchAction(ctx *cli.Context) error {
	cfgPath, err := filepath.Abs(ctx.String("c"))

	if err != nil {
		return err
	}

	os.Chdir(filepath.Dir(cfgPath))
	optsSetups := readCfg(cfgPath)

	pipeline := func(opts options) {
		purge(opts)
		cp(opts)
		build(opts)
		injectLR(opts)
		replace(opts)
	}

	for i := range optsSetups {
		opts := optsSetups[i]

		go func(opts options) {
			w := watcher.New()
			w.SetMaxEvents(1)
			w.FilterOps(watcher.Write, watcher.Rename, watcher.Move, watcher.Create, watcher.Remove)

			if len(opts.Watch.Exclude) > 0 {
				w.Ignore(opts.Watch.Exclude...)
			}

			if opts.ESBuild.Outdir != "" {
				w.Ignore(opts.ESBuild.Outdir)
			}

			for _, p := range opts.Watch.Paths {
				if err := w.AddRecursive(p); err != nil {
					fmt.Println(err.Error())
					os.Exit(1)
				}
			}

			go func() {
				for {
					select {
					case event := <-w.Event:
						fmt.Printf("File %s changed\n", event.Name())
						pipeline(opts)
					case err := <-w.Error:
						fmt.Println(err.Error())
					case <-w.Closed:
						return
					}
				}
			}()

			fmt.Printf("Watching %d elements in %s\n", len(w.WatchedFiles()), opts.Watch.Paths)

			pipeline(opts)

			if err := w.Start(time.Millisecond * 100); err != nil {
				fmt.Println(err.Error())
			}
		}(opts)

		if opts.Serve.Path != "" {
			go func() {
				port := 8080
				if opts.Serve.Port != 0 {
					port = opts.Serve.Port
				}

				err := Serve(opts.Serve.Path, uint(port))

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
		fmt.Println("Starting live reload server.")
		port := ctx.Uint("lr-port")
		lr := lrserver.New(lrserver.DefaultName, uint16(port))

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

	runProxy(ctx.Context, filepath.Dir(cfgPath), optsSetups)
	<-ctx.Done()
	fmt.Println("Stopped watching.")

	return nil
}
