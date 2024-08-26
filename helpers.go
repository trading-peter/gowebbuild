package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/otiai10/copy"
)

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
	result := api.Build(cfgToESBuildCfg(opts))

	if len(result.Errors) == 0 {
		triggerReload <- struct{}{}
	}
}

func getGoModuleName(root string) (string, error) {
	modFile := filepath.Join(root, "go.mod")

	if !isFile(modFile) {
		return "", fmt.Errorf("go.mod file not found")
	}

	// Open the go.mod file
	file, err := os.Open(modFile)
	if err != nil {
		return "", fmt.Errorf("error opening go.mod: %v", err)
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Iterate through the lines
	for scanner.Scan() {
		line := scanner.Text()
		// Check if the line starts with "module "
		if strings.HasPrefix(line, "module ") {
			// Extract the module name
			moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			return moduleName, nil
		}
	}

	// Check for errors in scanning
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning go.mod: %v", err)
	}

	return "", nil
}
