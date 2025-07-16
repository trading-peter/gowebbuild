package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/trading-peter/gowebbuild/fsutils"
	"github.com/urfave/cli/v2"
)

func buildAction(ctx *cli.Context) error {
	cfgPath := fsutils.ResolvePath(ctx.String("c"))

	os.Chdir(filepath.Dir(cfgPath))
	opts := readCfg(cfgPath)

	for _, o := range opts {
		if ctx.Bool("p") {
			download(o)
		}
		purge(o)
		cp(o)

		esBuildCfg := cfgToESBuildCfg(o)

		if ctx.Bool("p") {
			esBuildCfg.MinifyIdentifiers = true
			esBuildCfg.MinifySyntax = true
			esBuildCfg.MinifyWhitespace = true
			esBuildCfg.Sourcemap = api.SourceMapNone
		}

		esBuildCfg.Plugins = append(esBuildCfg.Plugins, contentSwapPlugin(o))

		api.Build(esBuildCfg)
		replace(o)

		if ctx.Bool("p") && o.ProductionBuildOptions.CmdPostBuild != "" {
			defer func() {
				fmt.Printf("Executing post production build command `%s`\n", o.ProductionBuildOptions.CmdPostBuild)
				cmd := exec.Command("sh", "-c", o.ProductionBuildOptions.CmdPostBuild)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()

				if err != nil {
					fmt.Printf("Failed to execute post production build command `%s`: %+v\n", o.ProductionBuildOptions.CmdPostBuild, err)
					os.Exit(1)
				}
			}()
		}
	}

	return nil
}
