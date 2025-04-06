package main

import (
	"context"
	"errors"
	"fmt"
	mobyterm "github.com/moby/term"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	remoteclient "k8s.io/client-go/tools/remotecommand"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubectl/pkg/util/term"
	"log"
	"net/url"
	"time"
)

func main() {
	runtimeClient, err := getRuntimeService(10)
	if err != nil {
		log.Fatalln(err)
	}

	sandboxSpec := "aaa.json"
	podSandboxConfig, err := loadPodSandboxConfig(sandboxSpec)
	if err != nil {
		log.Fatalln("load podSandboxConfig: %w", err)
	}

	// Test RuntimeServiceClient.RunPodSandbox
	podID, err := RunPodSandbox(runtimeClient, podSandboxConfig, defaultRuntimeEndpoints[0])
	if err != nil {
		log.Fatalf("run pod sandbox: %w", err)
	}
	fmt.Println(podID)

	imageClient, _ := getImageService()

	opts := &listOptions{
		//nameRegexp:         c.String("name"),
		//podID:              c.String("pod"),
		//podNamespaceRegexp: c.String("namespace"),
		//image:              c.String("image"),
		//state:              c.String("state"),
		//latest:             c.Bool("latest"),
		//last:               c.Int("last"),
		//all:                c.Bool("all"),
	}

	// c.StringSlice("label")
	slice, err := parseLabelStringSlice([]string{"xxx"})
	if err != nil {
		return
	}
	opts.labels = slice
	if err != nil {
		return
	}

	ctrs, err := ListContainers(runtimeClient, imageClient, opts)
	if err != nil {
		log.Fatalln("listing containers: %w", err)
	}

	for _, ctr := range ctrs {
		log.Println(ctr.GetId())
	}

	// ------

	execOpts := &execOptions{
		timeout:   10,
		tty:       true,
		stdin:     false,
		cmd:       []string{"ls", "/data/"},
		transport: "spdy", // websocket || spdy
	}

	execOpts.tlsConfig = &rest.TLSClientConfig{Insecure: true}

	id := "<cid>"
	optsCopy := &execOptions{
		id:        id,
		cmd:       execOpts.cmd,
		stdin:     execOpts.stdin,
		timeout:   execOpts.timeout,
		tlsConfig: execOpts.tlsConfig,
		transport: execOpts.transport,
		tty:       execOpts.tty,
	}

	if true {
		exitCode, err := ExecSync(runtimeClient, optsCopy)
		if err != nil {
			log.Printf("execing command in container %s synchronously: %w", id, err)
		}
		if exitCode != 0 {
			log.Fatalln("non-zero exit code", exitCode)
		}
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Exec(ctx, runtimeClient, optsCopy)
		if err != nil {
			log.Fatalln("execing command in container %s: %w", id, err)
		}
	}

}

const (
	// TODO: make this configurable in kubelet.
	kubeletURLSchema = "http"
	kubeletURLHost   = "http://127.0.0.1:10250"

	transportFlag      = "transport"
	transportWebsocket = "websocket"
	transportSpdy      = "spdy"

	detachSequence = "ctrl-p,ctrl-q"
)

func ExecSync(client internalapi.RuntimeService, opts *execOptions) (int, error) {
	request := &pb.ExecSyncRequest{
		ContainerId: opts.id,
		Cmd:         opts.cmd,
		Timeout:     opts.timeout,
	}
	logrus.Debugf("ExecSyncRequest: %v", request)

	timeoutDuration := time.Duration(opts.timeout) * time.Second

	type stdio struct {
		stdout, stderr []byte
	}

	io, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*stdio, error) {
		stdout, stderr, err := client.ExecSync(ctx, opts.id, opts.cmd, timeoutDuration)
		if err != nil {
			return nil, err
		}

		return &stdio{stdout, stderr}, nil
	})
	if err != nil {
		return 1, err
	}

	fmt.Println(string(io.stdout))
	fmt.Println(string(io.stderr))

	return 0, nil
}

// Exec sends an ExecRequest to server, and parses the returned ExecResponse.
func Exec(ctx context.Context, client internalapi.RuntimeService, opts *execOptions) error {
	request := &pb.ExecRequest{
		ContainerId: opts.id,
		Cmd:         opts.cmd,
		Tty:         opts.tty,
		Stdin:       opts.stdin,
		Stdout:      true,
		Stderr:      !opts.tty,
	}

	logrus.Debugf("ExecRequest: %v", request)

	r, err := InterruptableRPC(ctx, func(ctx context.Context) (*pb.ExecResponse, error) {
		return client.Exec(ctx, request)
	})
	logrus.Debugf("ExecResponse: %v", r)

	if err != nil {
		return err
	}

	execURL := r.Url

	URL, err := url.Parse(execURL)
	if err != nil {
		return err
	}

	if URL.Host == "" {
		URL.Host = kubeletURLHost
	}

	if URL.Scheme == "" {
		URL.Scheme = kubeletURLSchema
	}

	logrus.Debugf("Exec URL: %v", URL)

	return stream(ctx, opts.stdin, opts.tty, opts.transport, URL, opts.tlsConfig)
}

func stream(ctx context.Context, in, tty bool, transport string, parsedURL *url.URL, tlsConfig *rest.TLSClientConfig) error {
	executor, err := getExecutor(transport, parsedURL, tlsConfig)
	if err != nil {
		return fmt.Errorf("get executor: %w", err)
	}

	stdin, stdout, stderr := mobyterm.StdStreams()
	streamOptions := remoteclient.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	}

	if in {
		streamOptions.Stdin = stdin
	}

	logrus.Debugf("StreamOptions: %v", streamOptions)

	if !tty {
		return executor.StreamWithContext(ctx, streamOptions)
	}

	detachKeys, err := mobyterm.ToBytes(detachSequence)
	if err != nil {
		return errors.New("could not bind detach keys")
	}

	pr := mobyterm.NewEscapeProxy(streamOptions.Stdin, detachKeys)
	streamOptions.Stdin = pr

	if !in {
		return errors.New("tty=true must be specified with interactive=true")
	}

	t := term.TTY{
		In:  stdin,
		Out: stdout,
		Raw: true,
	}
	if !t.IsTerminalIn() {
		return errors.New("input is not a terminal")
	}

	streamOptions.TerminalSizeQueue = t.MonitorSize(t.GetSize())

	return t.Safe(func() error { return executor.StreamWithContext(ctx, streamOptions) })
}

func getExecutor(transport string, parsedURL *url.URL, tlsConfig *rest.TLSClientConfig) (exec remoteclient.Executor, err error) {
	config := &rest.Config{TLSClientConfig: *tlsConfig}

	switch transport {
	case transportSpdy:
		return remoteclient.NewSPDYExecutor(config, "POST", parsedURL)

	case transportWebsocket:
		return remoteclient.NewWebSocketExecutor(config, "GET", parsedURL.String())

	default:
		return nil, fmt.Errorf("unknown transport: %s", transport)
	}
}
