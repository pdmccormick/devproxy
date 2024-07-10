package devproxycmd

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/middleware"
)

const (
	DefaultHttpsBind = "127.0.0.1:8443"
)

func Main() {
	if err := Run(context.Background(), os.Args, os.Environ()); err != nil {
		log.Fatal(err)
	}
}

func Run(ctx context.Context, args, environ []string) error {
	// TODO add environment variable option to set flags

	var (
		fs          = flag.NewFlagSet(args[0], flag.ExitOnError)
		httpsFlag   = fs.String("https", DefaultHttpsBind, "run an HTTPS server on `addr:port`")
		noHttpsFlag = fs.Bool("nohttps", false, "do not run HTTPS server")
		httpFlag    = fs.String("http", "", "also run an HTTP on `addr:port`")

		pathMaps PathMaps
	)

	fs.Var(&pathMaps, "map", "add reverse proxy mapping `'/map/from/path = http://host/to/other/path'`")

	fs.Parse(args[1:])

	if err := pathMaps.FromEnviron(environ); err != nil {
		log.Fatal(err)
	}

	if *noHttpsFlag {
		*httpsFlag = ""
	}

	pm, err := NewProxyMap(pathMaps)
	if err != nil {
		log.Fatal(err)
	}

	mux := middleware.Logger(pm)

	var (
		runHttp  func() error
		runHttps func() error
	)

	if bind := *httpsFlag; bind != "" {
		tlsCert, err := makeCertificate()
		if err != nil {
			log.Fatalf("ERROR: failed to make certificate: %s", err)
		}

		l, err := net.Listen("tcp", bind)
		if err != nil {
			log.Fatalf("ERROR: unable to listen on `%s`: %s", bind, err)
		}

		var (
			tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
				MinVersion:   tls.VersionTLS13,
			}
			httpsServer = http.Server{
				Handler:   mux,
				TLSConfig: tlsConfig,
			}
			tlsListener = tls.NewListener(l, tlsConfig)
		)

		runHttps = func() error { return httpsServer.Serve(tlsListener) }
	}

	if bind := *httpFlag; bind != "" {
		l, err := net.Listen("tcp", bind)
		if err != nil {
			log.Fatalf("ERROR: unable to listen on `%s`: %s", bind, err)
		}

		var httpServer = http.Server{
			Handler: mux,
		}

		runHttp = func() error { return httpServer.Serve(l) }
	}

	var wg sync.WaitGroup

	for _, runFunc := range []func() error{runHttp, runHttps} {
		if runFunc == nil {
			continue
		}

		wg.Add(1)

		go func(run func() error) {
			defer wg.Done()

			if err := run(); err != nil {
				log.Fatalf("ERROR: %s", err)
			}
		}(runFunc)
	}

	wg.Wait()

	return nil
}
