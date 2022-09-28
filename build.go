package main

import (
	"os"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/urfave/cli/v2"
)

func buildAction(ctx *cli.Context) error {
	cfgPath := ctx.String("c")
	os.Chdir(filepath.Dir(cfgPath))
	opts := readCfg(cfgPath)

	for _, o := range opts {
		cp(o)

		if ctx.Bool("p") {
			o.ESBuild.MinifyIdentifiers = true
			o.ESBuild.MinifySyntax = true
			o.ESBuild.MinifyWhitespace = true
			o.ESBuild.Sourcemap = api.SourceMapNone
		}

		api.Build(o.ESBuild)
		replace(o)
	}

	return nil
}
