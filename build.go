package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/urfave/cli/v2"
)

func buildAction(ctx *cli.Context) error {
	cfgPath, err := filepath.Abs(ctx.String("c"))

	if err != nil {
		return err
	}

	os.Chdir(filepath.Dir(cfgPath))
	opts := readCfg(cfgPath)

	for _, o := range opts {
		if ctx.Bool("p") {
			download(o)
		}
		purge(o)
		cp(o)

		if ctx.Bool("p") {
			o.ESBuild.MinifyIdentifiers = true
			o.ESBuild.MinifySyntax = true
			o.ESBuild.MinifyWhitespace = true
			o.ESBuild.Sourcemap = api.SourceMapNone
		}

		api.Build(o.ESBuild.BuildOptions)
		replace(o)

		if ctx.Bool("p") && o.ProductionBuildOptions.CmdPostBuild != "" {
			fmt.Printf("Executing post production build command `%s`\n", o.ProductionBuildOptions.CmdPostBuild)
			cmd := exec.Command("sh", "-c", o.ProductionBuildOptions.CmdPostBuild)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()

			if err != nil {
				fmt.Printf("Failed to execute post production build command `%s`: %+v\n", o.ProductionBuildOptions.CmdPostBuild, err)
				os.Exit(1)
			}
		}
	}

	return nil
}
