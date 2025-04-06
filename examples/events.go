package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func Events(client internalapi.RuntimeService) error {
	errCh := make(chan error, 1)

	containerEventsCh := make(chan *pb.ContainerEventResponse)

	go func() {
		logrus.Debug("getting container events")

		_, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
			return nil, client.GetContainerEvents(ctx, containerEventsCh, nil)
		})
		if errors.Is(err, io.EOF) {
			errCh <- nil

			return
		}
		errCh <- err
	}()

	for {
		select {
		case err := <-errCh:
			return err
		case e := <-containerEventsCh:
			err := outputEvent(e, "json", "") // go-template 模式时可以传 go 模板语法
			if err != nil {
				fmt.Printf("failed to format container event with the error: %s\n", err)
			}
		}
	}
}
