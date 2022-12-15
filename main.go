package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/jaschaephraim/lrserver"
	"github.com/otiai10/copy"
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
		Version: "4.1.1",
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
			os.RemoveAll(opts.ESBuild.Outdir)
		}

		if opts.ESBuild.Outfile != "" {
			os.Remove(opts.ESBuild.Outfile)
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

			read, err := ioutil.ReadFile(p)
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
	result := api.Build(opts.ESBuild.BuildOptions)

	if len(result.Errors) == 0 {
		triggerReload <- struct{}{}
	}
}
