package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/radovskyb/watcher"
	"github.com/tidwall/gjson"
)

func link(from, to string) chan struct{} {
	requestBuildCh := make(chan struct{})

	// Load package.json in destination.
	destPkg := readFileContent(filepath.Join(to, "package.json"))
	depsRaw := gjson.Get(destPkg, "dependencies").Map()
	deps := map[string]bool{}
	for k := range depsRaw {
		deps[k] = true
	}

	packages := map[string]string{}
	packageFiles := findFiles(from, "package.json")

	for i := range packageFiles {
		content := readFileContent(packageFiles[i])
		name := gjson.Get(content, "name").String()

		if deps[name] {
			pp, err := filepath.Abs(filepath.Dir(packageFiles[i]))
			if err == nil {
				packages[name] = pp
			}
		}
	}

	fmt.Printf("Found %d npm packages to monitor for changes.\n", len(packages))

	go func() {
		w := watcher.New()
		w.SetMaxEvents(1)
		w.FilterOps(watcher.Write, watcher.Rename, watcher.Move, watcher.Create, watcher.Remove)
		w.IgnoreHiddenFiles(true)

		if err := w.AddRecursive(from); err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		go func() {
			for {
				select {
				case event := <-w.Event:
					fmt.Printf("File %s changed\n", event.Path)
					for k, v := range packages {
						if strings.HasPrefix(event.Path, v) {
							src := filepath.Dir(event.Path)
							dest := filepath.Join(to, "node_modules", k)
							fmt.Printf("Copying %s to %s\n", src, dest)
							err := copy.Copy(src, dest, copy.Options{
								Skip: func(stat fs.FileInfo, src, dest string) (bool, error) {
									if !isExcludedPath(src, "node_modules", ".git") && (stat.IsDir() || isIncludedExt(filepath.Base(src), "*.js", "*.ts")) {
										return false, nil
									}

									return true, nil
								},
								Sync: true,
							})

							if err != nil {
								fmt.Printf("Failed to copy %s: %v\n", k, err)
							}

							requestBuildCh <- struct{}{}
						}
					}
				case err := <-w.Error:
					fmt.Println(err.Error())
				case <-w.Closed:
					return
				}
			}
		}()

		fmt.Printf("Watching packages in %s\n", from)

		if err := w.Start(time.Millisecond * 100); err != nil {
			fmt.Println(err.Error())
		}
	}()

	return requestBuildCh
}

func isExcludedPath(srcPath string, exPaths ...string) bool {
	for _, exP := range exPaths {
		if strings.Contains(srcPath, exP) {
			return true
		}
	}
	return false
}

func isIncludedExt(srcPath string, extensions ...string) bool {
	for _, ext := range extensions {
		if ok, _ := filepath.Match(ext, srcPath); ok {
			return true
		}
	}

	return false
}

func findFiles(root, name string) []string {
	paths := []string{}

	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if !d.IsDir() && filepath.Base(path) == name && !strings.Contains(path, "node_modules") {
			paths = append(paths, path)
		}

		return nil
	})

	return paths
}

func readFileContent(path string) string {
	pkgData, err := os.ReadFile(path)

	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	return string(pkgData)
}
