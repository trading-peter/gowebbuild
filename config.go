package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/trading-peter/gowebbuild/fsutils"
	"gopkg.in/yaml.v3"
)

func cfgToESBuildCfg(cfg options) api.BuildOptions {
	return api.BuildOptions{
		EntryPoints: cfg.ESBuild.EntryPoints,
		Outdir:      cfg.ESBuild.Outdir,
		Outfile:     cfg.ESBuild.Outfile,
		Sourcemap:   api.SourceMap(cfg.ESBuild.Sourcemap),
		Format:      api.Format(cfg.ESBuild.Format),
		Splitting:   cfg.ESBuild.Splitting,
		Platform:    api.Platform(cfg.ESBuild.Platform),
		Bundle:      cfg.ESBuild.Bundle,
		Write:       cfg.ESBuild.Write,
		LogLevel:    api.LogLevel(cfg.ESBuild.LogLevel),
	}
}

type options struct {
	ESBuild struct {
		EntryPoints      []string `yaml:"entryPoints"`
		Outdir           string   `yaml:"outdir"`
		Outfile          string   `yaml:"outfile"`
		Sourcemap        int      `yaml:"sourcemap"`
		Format           int      `yaml:"format"`
		Splitting        bool     `yaml:"splitting"`
		Platform         int      `yaml:"platform"`
		Bundle           bool     `yaml:"bundle"`
		Write            bool     `yaml:"write"`
		LogLevel         int      `yaml:"logLevel"`
		PurgeBeforeBuild bool     `yaml:"purgeBeforeBuild"`
	} `yaml:"esbuild"`
	Watch struct {
		Paths   []string `yaml:"paths"`
		Exclude []string `yaml:"exclude"`
	}
	Serve struct {
		Path string `yaml:"path"`
		Port int    `yaml:"port"`
	} `yaml:"serve"`
	Copy []struct {
		Src  string `yaml:"src"`
		Dest string `yaml:"dest"`
	} `yaml:"copy"`
	Download []struct {
		Url  string `yaml:"url"`
		Dest string `yaml:"dest"`
	} `yaml:"download"`
	Replace []struct {
		Pattern string `yaml:"pattern"`
		Search  string `yaml:"search"`
		Replace string `yaml:"replace"`
	} `yaml:"replace"`
	Link struct {
		From string `yaml:"from"`
		To   string `yaml:"to"`
	} `yaml:"link"`
	ProductionBuildOptions struct {
		CmdPostBuild string `yaml:"cmdPostBuild"`
	} `yaml:"productionBuildOptions"`
	NpmProxy struct {
		Overrides []NpmProxyOverride
	} `yaml:"npm_proxy"`
}

type NpmProxyOverride struct {
	Namespace   string `yaml:"namespace"`
	Upstream    string `yaml:"upstream"`
	PackageRoot string `yaml:"packageRoot"`
}

func readCfg(cfgPath string) []options {
	if filepath.Ext(cfgPath) == ".json" {
		jsonOpts := readJsonCfg(cfgPath)

		data, err := yaml.Marshal(jsonOpts)
		if err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(1)
		}

		yamlPath := strings.TrimSuffix(cfgPath, ".json") + ".yaml"

		err = os.WriteFile(yamlPath, data, 0755)
		if err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(1)
		}

		cfgPath = yamlPath
	}

	cfgContent, err := os.ReadFile(cfgPath)

	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	optsSetups := []options{}

	err = yaml.Unmarshal(cfgContent, &optsSetups)
	if err != nil {
		opt := options{}
		err = yaml.Unmarshal(cfgContent, &opt)
		if err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(1)
		}

		optsSetups = append(optsSetups, opt)
	}

	// Process all paths in each options setup
	for i := range optsSetups {
		processPaths(&optsSetups[i])
	}

	return optsSetups
}

func readJsonCfg(cfgPath string) []options {
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

func processPaths(opts *options) {
	// ESBuild paths
	for i, entry := range opts.ESBuild.EntryPoints {
		opts.ESBuild.EntryPoints[i] = fsutils.ResolvePath(entry)
	}
	opts.ESBuild.Outdir = fsutils.ResolvePath(opts.ESBuild.Outdir)
	opts.ESBuild.Outfile = fsutils.ResolvePath(opts.ESBuild.Outfile)

	// Watch paths
	for i, path := range opts.Watch.Paths {
		opts.Watch.Paths[i] = fsutils.ResolvePath(path)
	}
	for i, path := range opts.Watch.Exclude {
		opts.Watch.Exclude[i] = fsutils.ResolvePath(path)
	}

	// Serve path
	opts.Serve.Path = fsutils.ResolvePath(opts.Serve.Path)

	// Copy paths
	for i := range opts.Copy {
		opts.Copy[i].Src = fsutils.ResolvePath(opts.Copy[i].Src)
		opts.Copy[i].Dest = fsutils.ResolvePath(opts.Copy[i].Dest)
	}

	// Download paths
	for i := range opts.Download {
		opts.Download[i].Dest = fsutils.ResolvePath(opts.Download[i].Dest)
	}

	// Link paths
	opts.Link.From = fsutils.ResolvePath(opts.Link.From)
	opts.Link.To = fsutils.ResolvePath(opts.Link.To)

	// Npm proxy paths
	for i := range opts.NpmProxy.Overrides {
		opts.NpmProxy.Overrides[i].PackageRoot = fsutils.ResolvePath(opts.NpmProxy.Overrides[i].PackageRoot)
	}
}
