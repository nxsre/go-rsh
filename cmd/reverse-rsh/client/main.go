package main

import (
	"code.cloudfoundry.org/tlsconfig"
	"flag"
	"github.com/nxsre/go-rsh"
	"log"
	"os"
)

var (
	addr            = flag.String("a", "127.0.0.1:22222,https://127.0.0.1:42222", "comma separated server addresses")
	shell           = flag.String("s", os.Getenv("SHELL"), "default shell to use")
	cacert          = flag.String("ca", "./certs/ca.pem", "ca certificate file")
	cert            = flag.String("cert", "./certs/client.pem", "server certificate file")
	key             = flag.String("key", "./certs/client-key.pem", "server key file")
	lastResortShell = "/bin/sh"
)

func parseArgs() {
	flag.Parse()

	if addr == nil || *addr == "" {
		log.Fatal("-a is required")
	}

	if shell == nil || *shell == "" {
		shell = &lastResortShell
	}
}

func main() {
	parseArgs()

	tlscfg, err := tlsconfig.Build(
		tlsconfig.WithIdentityFromFile(*cert, *key),
		tlsconfig.WithExternalServiceDefaults(),
		tlsconfig.WithInternalServiceDefaults(),
	).Client(tlsconfig.WithAuthorityFromFile(*cacert))

	if err != nil {
		log.Fatal(err)
	}
	server := rsh.NewReverseClient(*addr, *shell, tlscfg)
	if err := server.Serve(); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}
