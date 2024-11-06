package npmproxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/kataras/golog"
)

func (p *Proxy) externalHTTPServer(ctx context.Context) {
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    p.externalProxyHost,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown proxy server: %v\n", err)
		}
	}()

	mux.HandleFunc("/", p.incomingNpmRequestHandler)

	if err := srv.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}

// Receives incoming requests from the npm cli and decides based on override rules what server it should forwarded to.
func (p *Proxy) incomingNpmRequestHandler(res http.ResponseWriter, req *http.Request) {
	golog.Infof("Incoming NPM request for %s", req.URL.Path)
	pkgPath := strings.TrimLeft(req.URL.Path, "/")

	// If no matching override is found, we forward the request to the default registry.
	_, ok := p.matchingOverride(pkgPath)
	if !ok {
		serveReverseProxy(p.DefaultRegistry, res, req)
		return
	}

	// Process the override by forwarding the request to the internal proxy.

	serveReverseProxy(p.internalProxyUrl, res, req)

	// golog.Infof("Received request for url: %v", proxyUrl)
}

type ResponseWriterWrapper struct {
	http.ResponseWriter
	Body *bytes.Buffer
}

func (rw *ResponseWriterWrapper) Write(b []byte) (int, error) {
	rw.Body.Write(b)                  // Capture the response body
	return rw.ResponseWriter.Write(b) // Send the response to the original writer
}

func serveReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	// parse the OriginalUrl
	OriginalUrl, _ := url.Parse(target)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(OriginalUrl)

	// Update the headers to allow for SSL redirection
	req.URL.Host = OriginalUrl.Host
	req.URL.Scheme = OriginalUrl.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = OriginalUrl.Host

	wrappedRes := &ResponseWriterWrapper{ResponseWriter: res, Body: new(bytes.Buffer)}

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(wrappedRes, req)

	// Print the captured response body
	fmt.Println("Response body:", wrappedRes.Body.String())
}
