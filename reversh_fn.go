package rsh

import (
	"context"
	retry "github.com/avast/retry-go/v4"
	"github.com/jhump/grpctunnel"
	"github.com/jhump/grpctunnel/tunnelpb"
	"github.com/nxsre/go-rsh/pb"
	"google.golang.org/grpc/metadata"
	"k8s.io/klog/v2"
	"log"
	"time"
)

func tunnelRegister(ctx context.Context, conn *Connection) error {
	// 注册反向隧道，对 grpc server 端提供服务.
	tunnelStub := tunnelpb.NewTunnelServiceClient(conn)
	channelServer := grpctunnel.NewReverseTunnelServer(tunnelStub)

	// 注册 api
	pb.RegisterRemoteShellServer(channelServer, newRSHServer("/bin/sh"))

	klog.Infoln("Starting Client")
	// Create metadata and context.
	md := metadata.Pairs("client-id", GetNodeID())
	ctx = metadata.NewOutgoingContext(context.Background(), md)

	// Open the reverse tunnel and serve requests.
	err := retry.Do(
		func() error {
			_, err := channelServer.Serve(ctx)
			if err != nil {
				return err
			}
			return nil
		},
		retry.UntilSucceeded(), // 无限尝试
		retry.Delay(2*time.Second),
		retry.MaxDelay(5*time.Second),
		retry.MaxJitter(3*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			log.Println("retry", n, err)
		}),
	)
	return err
}
