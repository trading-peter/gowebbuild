package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jaschaephraim/lrserver"
	"github.com/radovskyb/watcher"
	"github.com/urfave/cli/v2"
)

var triggerReload = make(chan struct{})

func main() {
	cfgParam := &cli.StringFlag{
		Name:  "c",
		Value: "./.gowebbuild.yaml",
		Usage: "path to config file config file.",
	}

	app := &cli.App{
		Name:    "gowebbuild",
		Usage:   "All in one tool to build web frontend projects.",
		Version: "7.1.0",
		Authors: []*cli.Author{{
			Name: "trading-peter (https://github.com/trading-peter)",
		}},
		UsageText: `gowebbuild [global options] command [command options] [arguments...]

Examples:

Watch project and rebuild if a files changes:
$ gowebbuild

Use a different name or path for the config file (working directory is always the location of the config file):
$ gowebbuild -c /path/to/gowebbuild.yaml watch

Production build:
$ gowebbuild build -p

Manually replace a string within some files (not limited to project directory):
$ gowebbuild replace *.go foo bar
`,
		Commands: []*cli.Command{
			{
				Name:  "template",
				Usage: "execute a template",
				Flags: []cli.Flag{
					cfgParam,
				},
				Action: tplAction,
			},

			{
				Name:  "npm-proxy",
				Usage: "proxy npm packages",
				Flags: []cli.Flag{
					cfgParam,
				},
				Action: proxyAction,
			},

			{
				Name:  "build",
				Usage: "build web sources one time and exit",
				Flags: []cli.Flag{
					cfgParam,
					&cli.BoolFlag{
						Name:  "p",
						Value: false,
						Usage: "use production ready build settings",
					},
				},
				Action: buildAction,
			},

			{
				Name:  "watch",
				Usage: "watch for changes and trigger the build",
				Flags: []cli.Flag{
					cfgParam,
					&cli.UintFlag{
						Name:  "lr-port",
						Value: uint(lrserver.DefaultPort),
						Usage: "port for the live reload server",
					},
				},
				Action: watchAction,
			},

			{
				Name:  "serve",
				Usage: "serve a directory with a simply http server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "root",
						Value: "./",
						Usage: "folder to serve",
					},
					&cli.UintFlag{
						Name:  "port",
						Value: uint(8080),
						Usage: "serve directory this on port",
					},
					&cli.UintFlag{
						Name:  "lr-port",
						Value: uint(lrserver.DefaultPort),
						Usage: "port for the live reload server",
					},
				},
				Action: func(ctx *cli.Context) error {
					port := ctx.Uint("port")
					root := ctx.String("root")
					lrPort := ctx.Uint("lr-port")

					if lrPort != 0 {
						go func() {
							w := watcher.New()
							w.SetMaxEvents(1)
							w.FilterOps(watcher.Write, watcher.Rename, watcher.Move, watcher.Create, watcher.Remove)

							if err := w.AddRecursive(root); err != nil {
								fmt.Println(err.Error())
								os.Exit(1)
							}

							go func() {
								for {
									select {
									case event := <-w.Event:
										fmt.Printf("File %s changed\n", event.Name())
										triggerReload <- struct{}{}
									case err := <-w.Error:
										fmt.Println(err.Error())
									case <-w.Closed:
										return
									}
								}
							}()

							if err := w.Start(time.Millisecond * 100); err != nil {
								fmt.Println(err.Error())
							}
						}()

						go func() {
							lr := lrserver.New(lrserver.DefaultName, uint16(lrPort))

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
					}

					return Serve(root, port)
				},
			},

			{
				Name:  "download",
				Usage: "execute downloads as configured",
				Flags: []cli.Flag{
					cfgParam,
				},
				Action: func(ctx *cli.Context) error {
					cfgPath, err := filepath.Abs(ctx.String("c"))

					if err != nil {
						return err
					}

					os.Chdir(filepath.Dir(cfgPath))
					opts := readCfg(cfgPath)

					for i := range opts {
						download(opts[i])
					}
					return nil
				},
			},

			{
				Name:      "replace",
				ArgsUsage: "[glob file pattern] [search] [replace]",
				Usage:     "replace text in files",
				Action: func(ctx *cli.Context) error {
					files := ctx.Args().Get(0)
					searchStr := ctx.Args().Get(1)
					replaceStr := ctx.Args().Get(2)

					if files == "" {
						return fmt.Errorf("invalid file pattern")
					}

					if searchStr == "" {
						return fmt.Errorf("invalid search string")
					}

					replace(options{
						Replace: []struct {
							Pattern string `yaml:"pattern"`
							Search  string `yaml:"search"`
							Replace string `yaml:"replace"`
						}{
							{
								Pattern: files,
								Search:  searchStr,
								Replace: replaceStr,
							},
						},
					})
					return nil
				},
			},
		},
		DefaultCommand: "watch",
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	appCtx, cancel := context.WithCancel(context.Background())

	go func() {
		<-sigChan
		cancel()
		fmt.Println("Received interrupt, shutting down...")
	}()

	if err := app.RunContext(appCtx, os.Args); err != nil {
		fmt.Println(err)
	}
}
