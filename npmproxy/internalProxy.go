package npmproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/trading-peter/gowebbuild/fsutils"
)

func (p *Proxy) internalHTTPServer(ctx context.Context) {
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    p.internalProxyHost,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown internal server for npm proxy: %v\n", err)
		}
	}()

	mux.HandleFunc("GET /{pkg}", func(w http.ResponseWriter, r *http.Request) {
		pkgName := r.PathValue("pkg")
		override, ok := p.matchingOverride(pkgName)

		if !ok {
			http.NotFound(w, r)
			return
		}

		pkg, err := p.findPackageSource(override, pkgName)
		if err != nil {
			serveReverseProxy(override.Upstream, w, r)
			return
		}

		json.NewEncoder(w).Encode(pkg)
	})

	mux.HandleFunc("GET /files/{file}", func(w http.ResponseWriter, r *http.Request) {
		fileName := r.PathValue("file")
		filePath := filepath.Join(p.pkgCachePath, fileName)

		if !fsutils.IsFile(filePath) {
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, filePath)
	})

	if err := srv.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}
