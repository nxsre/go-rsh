package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	"net/http"
	"net/url"
	"os"
)

// PortForward sends an PortForwardRequest to server, and parses the returned PortForwardResponse.
func PortForward(client internalapi.RuntimeService, opts portforwardOptions) error {
	if opts.id == "" {
		return errors.New("ID cannot be empty")
	}

	request := &pb.PortForwardRequest{
		PodSandboxId: opts.id,
	}
	logrus.Debugf("PortForwardRequest: %v", request)

	r, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.PortForwardResponse, error) {
		return client.PortForward(ctx, request)
	})
	logrus.Debugf("PortForwardResponse; %v", r)

	if err != nil {
		return err
	}

	parsedURL, err := url.Parse(r.Url)
	if err != nil {
		return err
	}

	if parsedURL.Host == "" {
		parsedURL.Host = kubeletURLHost
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = kubeletURLSchema
	}

	logrus.Debugf("PortForward URL: %v", parsedURL)

	dialer, err := getDialer(opts.transport, parsedURL, opts.tlsConfig)
	if err != nil {
		return fmt.Errorf("get dialer: %w", err)
	}

	readyChan := make(chan struct{})

	logrus.Debugf("Ports to forward: %v", opts.ports)

	pf, err := portforward.New(dialer, opts.ports, SetupInterruptSignalHandler(), readyChan, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	return pf.ForwardPorts()
}

func getDialer(transport string, parsedURL *url.URL, tlsConfig *rest.TLSClientConfig) (exec httpstream.Dialer, err error) {
	config := &rest.Config{TLSClientConfig: *tlsConfig}

	switch transport {
	case transportSpdy:
		tr, upgrader, err := spdy.RoundTripperFor(config)
		if err != nil {
			return nil, fmt.Errorf("get SPDY round tripper: %w", err)
		}

		return spdy.NewDialer(upgrader, &http.Client{Transport: tr}, "POST", parsedURL), nil

	case transportWebsocket:
		return portforward.NewSPDYOverWebsocketDialer(parsedURL, config)

	default:
		return nil, fmt.Errorf("unknown transport: %s", transport)
	}
}
