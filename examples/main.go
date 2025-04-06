package main

import (
	"errors"
	"github.com/sirupsen/logrus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	internalapi "k8s.io/cri-api/pkg/apis"
	remote "k8s.io/cri-client/pkg"
	"k8s.io/klog/v2"
	"runtime"
	"sigs.k8s.io/cri-tools/pkg/framework"
	"time"
)

const (
	defaultTimeout        = 2 * time.Second
	defaultTimeoutWindows = 200 * time.Second
)

var (
	// RuntimeEndpoint is CRI server runtime endpoint.
	RuntimeEndpoint string
	// RuntimeEndpointIsSet is true when RuntimeEndpoint is configured.
	RuntimeEndpointIsSet bool
	// ImageEndpoint is CRI server image endpoint, default same as runtime endpoint.
	ImageEndpoint string
	// ImageEndpointIsSet is true when ImageEndpoint is configured.
	ImageEndpointIsSet bool
	// Timeout  of connecting to server (default: 2s on Linux, 200s on Windows).
	Timeout time.Duration
	// Debug enable debug output.
	Debug bool
	// PullImageOnCreate enables pulling image on create requests.
	PullImageOnCreate bool
	// DisablePullOnRun disable pulling image on run requests.
	DisablePullOnRun bool
	// tracerProvider is the global OpenTelemetry tracing instance.
	tracerProvider *sdktrace.TracerProvider
)

func getRuntimeService(timeout time.Duration) (res internalapi.RuntimeService, err error) {
	if RuntimeEndpointIsSet && RuntimeEndpoint == "" {
		return nil, errors.New("--runtime-endpoint is not set")
	}

	logrus.Debug("Get runtime connection")

	// Check if a custom timeout is provided.
	t := Timeout
	if timeout != 0 {
		t = timeout
	}

	logrus.Debugf("Using runtime connection timeout: %v", t)

	// Use the noop tracer provider and not tracerProvider directly, otherwise
	// we'll panic in the unary call interceptor
	var tp trace.TracerProvider = noop.NewTracerProvider()
	if tracerProvider != nil {
		tp = tracerProvider
	}

	logger := klog.Background()

	// If no EP set then use the default endpoint types
	if !RuntimeEndpointIsSet {
		logrus.Warningf("runtime connect using default endpoints: %v. "+
			"As the default settings are now deprecated, you should set the "+
			"endpoint instead.", defaultRuntimeEndpoints)
		logrus.Debug("Note that performance maybe affected as each default " +
			"connection attempt takes n-seconds to complete before timing out " +
			"and going to the next in sequence.")

		for _, endPoint := range defaultRuntimeEndpoints {
			logrus.Debugf("Connect using endpoint %q with %q timeout", endPoint, t)

			res, err = remote.NewRemoteRuntimeService(endPoint, t, tp, &logger)
			if err != nil {
				logrus.Error(err)

				continue
			}

			logrus.Debugf("Connected successfully using endpoint: %s", endPoint)

			break
		}

		return res, err
	}

	return remote.NewRemoteRuntimeService(RuntimeEndpoint, t, tp, &logger)
}

func getImageService() (res internalapi.ImageManagerService, err error) {
	if ImageEndpoint == "" {
		if RuntimeEndpointIsSet && RuntimeEndpoint == "" {
			return nil, errors.New("--image-endpoint is not set")
		}

		ImageEndpoint = RuntimeEndpoint
		ImageEndpointIsSet = RuntimeEndpointIsSet
	}

	logrus.Debug("Get image connection")

	// Use the noop tracer provider and not tracerProvider directly, otherwise
	// we'll panic in the unary call interceptor
	var tp trace.TracerProvider = noop.NewTracerProvider()
	if tracerProvider != nil {
		tp = tracerProvider
	}

	logger := klog.Background()

	// If no EP set then use the default endpoint types
	if !ImageEndpointIsSet {
		logrus.Warningf("Image connect using default endpoints: %v. "+
			"As the default settings are now deprecated, you should set the "+
			"endpoint instead.", defaultRuntimeEndpoints)
		logrus.Debug("Note that performance maybe affected as each default " +
			"connection attempt takes n-seconds to complete before timing out " +
			"and going to the next in sequence.")

		for _, endPoint := range defaultRuntimeEndpoints {
			logrus.Debugf("Connect using endpoint %q with %q timeout", endPoint, Timeout)

			res, err = remote.NewRemoteImageService(endPoint, Timeout, tp, &logger)
			if err != nil {
				logrus.Error(err)

				continue
			}

			logrus.Debugf("Connected successfully using endpoint: %s", endPoint)

			break
		}

		return res, err
	}

	return remote.NewRemoteImageService(ImageEndpoint, Timeout, tp, &logger)
}

func getTimeout(timeDuration time.Duration) time.Duration {
	if timeDuration.Seconds() > 0 {
		return timeDuration
	}

	if runtime.GOOS == framework.OSWindows {
		return defaultTimeoutWindows
	}

	return defaultTimeout // use default
}
