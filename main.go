package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/jaschaephraim/lrserver"
	"github.com/otiai10/copy"
	"github.com/radovskyb/watcher"
	"github.com/urfave/cli/v2"
)

var triggerReload = make(chan struct{})

type ESBuildExtended struct {
	api.BuildOptions
	PurgeBeforeBuild bool
}

type options struct {
	ESBuild ESBuildExtended
	Watch   struct {
		Path    string
		Exclude []string
	}
	Serve struct {
		Path string
		Port int
	}
	Copy []struct {
		Src  string
		Dest string
	}
	Download []struct {
		Url  string
		Dest string
	}
	Replace []struct {
		Pattern string
		Search  string
		Replace string
	}
	Link struct {
		From string
		To   string
	}
}

func readCfg(cfgPath string) []options {
	cfgContent, err := os.ReadFile(cfgPath)

	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	optsSetups := []options{}

	err = json.Unmarshal(cfgContent, &optsSetups)
	if err != nil {
		opt := options{}
		err = json.Unmarshal(cfgContent, &opt)
		if err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(1)
		}

		optsSetups = append(optsSetups, opt)
	}

	return optsSetups
}

func main() {
	cfgParam := &cli.StringFlag{
		Name:  "c",
		Value: "./.gowebbuild.json",
		Usage: "path to config file config file.",
	}

	app := &cli.App{
		Name:    "gowebbuild",
		Usage:   "All in one tool to build web frontend projects.",
		Version: "4.3.0",
		Authors: []*cli.Author{{
			Name: "trading-peter (https://github.com/trading-peter)",
		}},
		UsageText: `gowebbuild [global options] command [command options] [arguments...]

Examples:

Watch project and rebuild if a files changes:
$ gowebbuild

Use a different name or path for the config file (working directory is always the location of the config file):
$ gowebbuild -c watch

Production build:
$ gowebbuild build -p

Manually replace a string within some files (not limited to project directory):
$ gowebbuild replace *.go foo bar
`,
		Commands: []*cli.Command{
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
							Pattern string
							Search  string
							Replace string
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

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
	}
}

func purge(opts options) {
	if opts.ESBuild.PurgeBeforeBuild {
		if opts.ESBuild.Outdir != "" {
			fmt.Printf("Purging output folder %s\n", opts.ESBuild.Outdir)
			os.RemoveAll(opts.ESBuild.Outdir)
		}

		if opts.ESBuild.Outfile != "" {
			fmt.Printf("Purging output file %s\n", opts.ESBuild.Outfile)
			os.Remove(opts.ESBuild.Outfile)
		}
	}
}

func download(opts options) {
	if len(opts.Download) == 0 {
		return
	}

	for _, dl := range opts.Download {
		if !isDir(filepath.Dir(dl.Dest)) {
			fmt.Printf("Failed to find destination folder for downloading from %s\n", dl.Url)
			continue
		}

		file, err := os.Create(dl.Dest)
		if err != nil {
			fmt.Printf("Failed to create file for downloading from %s: %v\n", dl.Url, err)
			continue
		}
		defer file.Close()

		client := http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
		}

		fmt.Printf("Downloading %s to %s\n", dl.Url, dl.Dest)
		resp, err := client.Get(dl.Url)
		if err != nil {
			fmt.Printf("Failed to download file from %s: %v\n", dl.Url, err)
			continue
		}
		defer resp.Body.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			fmt.Printf("Failed to write file downloaded from %s: %v\n", dl.Url, err)
			continue
		}
	}
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

		for _, p := range paths {
			if !isFile(p) {
				continue
			}

			read, err := os.ReadFile(p)
			if err != nil {
				fmt.Printf("%+v\n", err)
				os.Exit(1)
			}

			r := op.Replace
			if strings.HasPrefix(op.Replace, "$") {
				r = os.ExpandEnv(op.Replace)
			}

			count := strings.Count(string(read), op.Search)

			if count > 0 {
				fmt.Printf("Replacing %d occurrences of '%s' with '%s' in %s\n", count, op.Search, r, p)
				newContents := strings.ReplaceAll(string(read), op.Search, r)
				err = os.WriteFile(p, []byte(newContents), 0)

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
	result := api.Build(opts.ESBuild.BuildOptions)

	if len(result.Errors) == 0 {
		triggerReload <- struct{}{}
	}
}
