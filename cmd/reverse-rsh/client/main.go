package main

import (
	"code.cloudfoundry.org/tlsconfig"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ibice/go-rsh"
)

var (
	port            = flag.Uint("p", 22222, "listen port")
	addr            = flag.String("a", "127.0.0.1", "listen address")
	shell           = flag.String("s", os.Getenv("SHELL"), "default shell to use")
	cacert          = flag.String("ca", "./certs/ca.pem", "ca certificate file")
	cert            = flag.String("cert", "./certs/client.pem", "server certificate file")
	key             = flag.String("key", "./certs/client-key.pem", "server key file")
	lastResortShell = "/bin/sh"
)

func parseArgs() {
	flag.Parse()

	if port == nil || *port == 0 {
		log.Fatal("-p is required")
	}

	if *port > 65535 {
		log.Fatal("Invalid port: ")
	}

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
	).Client(tlsconfig.WithAuthorityFromFile(*cacert), tlsconfig.WithServerName(*addr))
	if err != nil {
		log.Fatal(err)
	}
	server := rsh.NewReverseClient(fmt.Sprintf("%s:%d", *addr, *port), *shell, tlscfg)
	if err := server.Serve(); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}
