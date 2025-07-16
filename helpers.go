package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/otiai10/copy"
	"github.com/trading-peter/gowebbuild/fsutils"
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
		if !fsutils.IsDir(filepath.Dir(dl.Dest)) {
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

		destIsDir := fsutils.IsDir(op.Dest)
		for _, p := range paths {
			d := op.Dest

			if destIsDir && fsutils.IsFile(p) {
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
			if !fsutils.IsFile(p) {
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

func injectLR(opts options) {
	if opts.Watch.InjectLiveReload == "" {
		return
	}

	// Read the HTML file
	contents, err := os.ReadFile(opts.Watch.InjectLiveReload)
	if err != nil {
		fmt.Printf("Failed to read inject live reload script: %v\n", err)
		return
	}

	htmlContent := string(contents)

	if !opts.Watch.SkipCSPInject {
		// First modify CSP
		htmlContent, err = updateContentPolicyTag(htmlContent)
		if err != nil {
			fmt.Println("Error modifying CSP:", err)
			return
		}
	}

	// Then inject script
	finalHTML, err := injectLiveReloadScript(htmlContent)
	if err != nil {
		fmt.Println("Error injecting script:", err)
		return
	}

	err = os.WriteFile(opts.Watch.InjectLiveReload, []byte(finalHTML), 0644)
	if err != nil {
		fmt.Printf("Failed to write live reload script reference: %v\n", err)
		return
	}

	fmt.Printf("Injected live reload script reference into %s\n", opts.Watch.InjectLiveReload)
}

func injectLiveReloadScript(html string) (string, error) {
	// Check if script is already present
	if strings.Contains(html, "livereload.js") {
		return html, nil
	}

	// Find the closing body tag and inject script before it
	bodyCloseRegex := regexp.MustCompile(`(?i)</body>`)
	if !bodyCloseRegex.MatchString(html) {
		return html, nil // Return unchanged if no body tag found
	}

	scriptTag := `<script src="http://localhost:35729/livereload.js" type="text/javascript"></script>`
	newHTML := bodyCloseRegex.ReplaceAllString(html, scriptTag+"</body>")

	return newHTML, nil
}

func updateContentPolicyTag(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html, err
	}

	liveReloadHost := "localhost:35729"
	liveReloadURL := "http://" + liveReloadHost
	liveReloadWS := "ws://" + liveReloadHost

	doc.Find("meta[http-equiv='Content-Security-Policy']").Each(func(i int, s *goquery.Selection) {
		if originalCSP, ok := s.Attr("content"); ok {
			// Split CSP into individual directives
			directives := strings.Split(originalCSP, ";")

			// Look for script-src directive
			scriptSrcFound := false
			connectSrcFound := false

			for i, directive := range directives {
				trimmed := strings.TrimSpace(directive)

				// Handle script-src directive
				if strings.HasPrefix(trimmed, "script-src") {
					// If script-src already exists, append localhost if not present
					if !strings.Contains(trimmed, liveReloadURL) {
						directives[i] = trimmed + " " + liveReloadURL
					}
					scriptSrcFound = true
				}

				// Handle connect-src directive
				if strings.HasPrefix(trimmed, "connect-src") {
					// If connect-src already exists, append WebSocket URL if not present
					if !strings.Contains(trimmed, liveReloadWS) {
						directives[i] = trimmed + " " + liveReloadWS
					}
					connectSrcFound = true
				}
			}

			// If no script-src found, add it with 'self' as default
			if !scriptSrcFound {
				directives = append(directives, "script-src 'self' "+liveReloadURL)
			}

			// If no connect-src found, add it with 'self' as default
			if !connectSrcFound {
				directives = append(directives, "connect-src 'self' "+liveReloadWS)
			}

			// Join directives back together
			newCSP := strings.Join(directives, ";")

			// Ensure we don't have trailing semicolon if original didn't
			if !strings.HasSuffix(originalCSP, ";") && strings.HasSuffix(newCSP, ";") {
				newCSP = strings.TrimSuffix(newCSP, ";")
			}

			s.SetAttr("content", newCSP)
		}
	})

	var buf bytes.Buffer
	err = goquery.Render(&buf, doc.Selection)
	if err != nil {
		return html, err
	}

	return buf.String(), nil
}

func build(opts options) {
	esBuildOpts := cfgToESBuildCfg(opts)

	esBuildOpts.Plugins = append(esBuildOpts.Plugins, contentSwapPlugin(opts))

	result := api.Build(esBuildOpts)

	if len(result.Errors) == 0 {
		triggerReload <- struct{}{}
	}
}

func contentSwapPlugin(opts options) api.Plugin {
	return api.Plugin{
		Name: "content-swap",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `.*`},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					for _, swap := range opts.ContentSwap {
						if strings.HasSuffix(args.Path, swap.File) {
							fmt.Printf("Swapping content of %s with %s\n", args.Path, swap.ReplaceWith)

							text, err := os.ReadFile(swap.ReplaceWith)
							if err != nil {
								return api.OnLoadResult{}, err
							}
							contents := string(text)
							return api.OnLoadResult{
								Contents: &contents,
								Loader:   api.LoaderJS,
							}, nil
						}
					}
					return api.OnLoadResult{}, nil
				})
		},
	}
}

func getGoModuleName(root string) (string, error) {
	modFile := filepath.Join(root, "go.mod")

	if !fsutils.IsFile(modFile) {
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

func findFreePort(from, to int) int {
	for port := from; port <= to; port++ {
		if isFreePort(port) {
			return port
		}
		port++
	}

	return -1
}

func isFreePort(port int) bool {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return false
	}
	defer l.Close()
	return true
}
