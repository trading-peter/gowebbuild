package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/trading-peter/gowebbuild/npmproxy"
	"github.com/urfave/cli/v2"
)

func proxyAction(ctx *cli.Context) error {
	cfgPath, err := filepath.Abs(ctx.String("c"))

	if err != nil {
		return err
	}

	projectDir := filepath.Dir(cfgPath)
	os.Chdir(projectDir)
	opts := readCfg(cfgPath)

	return runProxy(ctx.Context, projectDir, opts)
}

func runProxy(ctx context.Context, projectDir string, opts []options) error {
	overrides := []NpmProxyOverride{}

	for _, o := range opts {
		overrides = append(overrides, o.NpmProxy.Overrides...)
	}

	if len(overrides) == 0 {
		return nil
	}

	fmt.Printf("Found %d npm overrides. Starting proxy server.\n", len(overrides))

	// if fs.IsFile(filepath.Join(projectDir, ".npmrc")) {
	// 	return fmt.Errorf(".npmrc file already exists in project root.")
	// }

	freePort := findFreePort(10000, 20000)
	freePortInternal := findFreePort(20001, 30000)

	if freePort == -1 || freePortInternal == -1 {
		return fmt.Errorf("Failed to find free ports for proxy setup.")
	}

	list := []npmproxy.Override{}
	npmrcRules := []string{
		";CREATED BY GOWEBBUILD. DO NOT EDIT.",
		";This file is used by the npm proxy server.",
		";It is used to override the default registry for specific package namespaces.",
		";This file will be removed after the proxy server is stopped.",
	}

	for _, o := range overrides {
		list = append(list, npmproxy.Override{
			Namespace:   o.Namespace,
			Upstream:    o.Upstream,
			PackageRoot: o.PackageRoot,
		})

		npmrcRules = append(npmrcRules, fmt.Sprintf("%s:registry=http://localhost:%d", o.Namespace, freePort))
	}

	err := os.WriteFile(filepath.Join(projectDir, ".npmrc"), []byte(strings.Join(npmrcRules, "\n")), 0644)
	if err != nil {
		return err
	}

	defer os.Remove(filepath.Join(projectDir, ".npmrc"))

	proxy := npmproxy.New(
		list,
		projectDir,
		npmproxy.WithPort(freePort),
		npmproxy.WithInternalPort(freePortInternal),
	)

	proxy.Start(ctx)
	fmt.Println("Stopped npm proxy server")
	return nil
}
