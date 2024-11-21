package fsutils

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func FindFiles(root, name string) []string {
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

func IsFile(path string) bool {
	stat, err := os.Stat(path)

	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	return !stat.IsDir()
}

func IsDir(path string) bool {
	stat, err := os.Stat(path)

	if errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(path, 0755)
		return true
	}

	return err == nil && stat.IsDir()
}

func ResolvePath(path string) string {
	// We assume that the user doesn't use the involved feature if the path is empty.
	if path == "" {
		return ""
	}

	expandedPath := os.ExpandEnv(path)

	if strings.HasPrefix(expandedPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		expandedPath = filepath.Join(homeDir, expandedPath[1:])
	}

	path, err := filepath.Abs(expandedPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	return path
}
