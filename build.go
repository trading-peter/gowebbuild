package main

import (
	"os"
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
	}

	return nil
}
