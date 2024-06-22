package devproxycmd

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
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
		httpsFlag   = fs.String("https", ":8443", "run an HTTPS server on `addr:port`")
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

	// TODO add HTTPS support as default
	if err := http.ListenAndServe(*httpFlag, pm); err != nil {
		log.Fatal(err)
	}

	return nil
}
