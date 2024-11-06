package npmproxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Override struct {
	Namespace   string
	Upstream    string
	PackageRoot string
}

type Proxy struct {
	ProjectRoot       string
	Port              int
	InternalPort      int
	DefaultRegistry   string
	Overrides         []Override
	pkgCachePath      string
	externalProxyHost string
	internalProxyHost string
	internalProxyUrl  string
}

type ProxyOption func(*Proxy)

func WithPort(port int) ProxyOption {
	return func(p *Proxy) {
		p.Port = port
	}
}

func WithInternalPort(port int) ProxyOption {
	return func(p *Proxy) {
		p.InternalPort = port
	}
}

func WithPkgCachePath(path string) ProxyOption {
	return func(p *Proxy) {
		p.pkgCachePath = path
	}
}

func WithDefaultRegistry(registry string) ProxyOption {
	return func(p *Proxy) {
		p.DefaultRegistry = strings.TrimSuffix(registry, "/")
	}
}

func New(overrides []Override, projectRoot string, options ...ProxyOption) *Proxy {
	p := &Proxy{
		ProjectRoot:     projectRoot,
		Port:            1234,
		InternalPort:    1235,
		DefaultRegistry: "https://registry.npmjs.org",
		Overrides:       overrides,
	}

	for _, option := range options {
		option(p)
	}

	if p.pkgCachePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		p.pkgCachePath = filepath.Join(homeDir, ".gowebbuild", "proxy", "cache")
	}

	p.externalProxyHost = fmt.Sprintf("127.0.0.1:%d", p.Port)
	p.internalProxyHost = fmt.Sprintf("127.0.0.1:%d", p.InternalPort)
	p.internalProxyUrl = fmt.Sprintf("http://%s", p.internalProxyHost)

	return p
}

func (p *Proxy) Start(ctx context.Context) {
	go p.internalHTTPServer(ctx)
	p.externalHTTPServer(ctx)
}

func (p *Proxy) matchingOverride(path string) (*Override, bool) {
	for _, o := range p.Overrides {
		if strings.HasPrefix(path, o.Namespace) {
			return &o, true
		}
	}
	return nil, false
}
