package devproxycmd

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"slices"
	"strings"
)

type PathMaps []*PathMap

var _ flag.Value = (*PathMaps)(nil)

func (pms PathMaps) String() string {
	var strs []string
	for _, p := range pms {
		var s = fmt.Sprintf("%s = %s", p.Path, p.Target)
		strs = append(strs, s)
	}

	return strings.Join(strs, "\n")
}

func (pms *PathMaps) Set(arg string) error {
	var parts = strings.SplitN(arg, "=", 2)

	if len(parts) < 2 {
		return fmt.Errorf("missing '=' separator")
	}

	var (
		srcPath   = strings.TrimSpace(parts[0])
		targetStr = strings.TrimSpace(parts[1])
	)

	if srcPath == "" {
		return fmt.Errorf("path cannot be empty")
	}

	target, err := url.Parse(targetStr)
	if err != nil {
		return fmt.Errorf("bad URL: %w", err)
	}

	if target.Path == "" {
		target.Path = "/"
	}

	switch target.Scheme {
	case "":
		return fmt.Errorf("URL cannot have empty scheme, must be either `http` or `https` scheme")
	case "http", "https":
	default:
		return fmt.Errorf("unrecognized URL scheme `%s`, must be either `http` or `https` scheme", target.Scheme)
	}

	if h := target.Host; h == "" || strings.HasPrefix(h, ":") {
		return fmt.Errorf("missing URL host")
	}

	if strings.HasSuffix(targetStr, "/") && !strings.HasSuffix(target.Path, "/") {
		target.Path += "/"
	}

	var path = path.Clean(srcPath)

	if strings.HasSuffix(srcPath, "/") && !strings.HasSuffix(path, "/") {
		path += "/"
	}

	var pm = PathMap{
		Path:   path,
		Target: target,
	}

	*pms = append(*pms, &pm)

	return nil
}

const (
	EnvironPrefixOne  = `DEVPROXY_MAP`
	EnvironPrefixMany = `DEVPROXY_MAP_`
)

func (pms *PathMaps) FromEnviron(environ []string) error {
	for _, line := range environ {
		var keypair = strings.SplitN(line, "=", 2)

		if len(keypair) < 2 {
			continue
		}

		var (
			key   = strings.TrimSpace(keypair[0])
			value = strings.TrimSpace(keypair[1])
		)

		if key == EnvironPrefixOne || strings.HasPrefix(key, EnvironPrefixMany) {
			if err := pms.Set(value); err != nil {
				return fmt.Errorf("Environ `%s`: %w", key, err)
			}
		}
	}

	return nil
}

// A mapping between a path and a destination URL
type PathMap struct {
	Path   string
	Target *url.URL
	Proxy  *httputil.ReverseProxy
}

var _ http.Handler = (*PathMap)(nil)

func (pm *PathMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.RequestURI = strings.TrimPrefix(r.RequestURI, pm.Path)
	pm.Proxy.ServeHTTP(w, r)
}

type ProxyMap struct {
	Paths           map[string]*PathMap
	Transports      map[string]*http.Transport
	Mux             *http.ServeMux
	TLSClientConfig tls.Config
}

var _ http.Handler = (*ProxyMap)(nil)

func (pm *ProxyMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pm.Mux.ServeHTTP(w, r)
}

func NewProxyMap(paths PathMaps) (*ProxyMap, error) {
	var pm = ProxyMap{
		Paths:      make(map[string]*PathMap),
		Transports: make(map[string]*http.Transport),
		Mux:        http.NewServeMux(),
		TLSClientConfig: tls.Config{
			InsecureSkipVerify: true,
		},
	}

	for _, p := range paths {
		pm.Paths[p.Path] = p
	}

	for path, p := range pm.Paths {
		var slashSuffix = strings.HasSuffix(path, "/")
		if !slashSuffix {
			if pathSlash := path + "/"; pm.Paths[pathSlash] == nil {
				pm.Paths[pathSlash] = p
			}
		}
	}

	var pathSet = make([]string, 0, len(pm.Paths))

	for path, _ := range pm.Paths {
		pathSet = append(pathSet, path)
	}

	slices.Sort(pathSet)

	for _, path := range pathSet {
		p := pm.Paths[path]
		fmt.Printf("    %-10s => %s\n", path, p.Target)
	}

	for path, p := range pm.Paths {
		pm.handle(path, p)

		/*
			var slashSuffix = strings.HasSuffix(path, "/")
			if !slashSuffix {
				if pathSlash := path + "/"; pm.Paths[pathSlash] == nil {
					pm.handle(pathSlash, p)
				}
			}
		*/
	}

	for _, p := range pm.Paths {
		var (
			baseTarget = url.URL{
				Scheme: p.Target.Scheme,
				User:   p.Target.User,
				Host:   p.Target.Host,
			}
			transportKey = baseTarget.String()
		)

		transport, ok := pm.Transports[transportKey]
		if !ok {
			transport = &http.Transport{
				TLSClientConfig: &pm.TLSClientConfig,
			}
			pm.Transports[transportKey] = transport
		}

		p.Proxy = &httputil.ReverseProxy{
			Transport: transport,
			Rewrite: func(r *httputil.ProxyRequest) {
				r.Out.Host = p.Target.Host
				r.SetXForwarded()

				var inURL = r.In.URL

				targetPath, err := url.JoinPath(p.Target.Path, inURL.Path)
				if err != nil {
					log.Printf("JoinPath error: %s", err)
				}

				var outURL = url.URL{
					Scheme:   p.Target.Scheme,
					User:     p.Target.User,
					Host:     p.Target.Host,
					Path:     targetPath,
					RawQuery: inURL.RawQuery,
				}

				r.Out.URL = &outURL
				r.Out.RequestURI = outURL.RequestURI()
			},
		}

	}

	return &pm, nil
}

func (pm *ProxyMap) handle(path string, p *PathMap) {
	var h http.Handler = p
	if path != "/" {
		h = http.StripPrefix(strings.TrimSuffix(path, "/"), p)
	}

	pm.Mux.Handle(path, h)
}
