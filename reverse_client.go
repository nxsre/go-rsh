package rsh

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// ReverseClient is the local shell server.
type ReverseClient struct {
	address   string
	shell     string
	tlsconfig *tls.Config
}

// NewReverseClient creates a new local shell client.
func NewReverseClient(address string, shell string, tlcfg *tls.Config) *ReverseClient {
	return &ReverseClient{
		address:   address,
		shell:     shell,
		tlsconfig: tlcfg,
	}
}

func (s *ReverseClient) Dialer() func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, deviceID string) (net.Conn, error) {
		fmt.Println("reverse dialer deviceID: " + deviceID)
		// 建立连接
		// 不能用 tls dial，会出现："transport: authentication handshake failed: EOF"
		// grpc 的 tls 连接是在 NewClient 时 grpc.WithTransportCredentials 配置的，dial 只返回 tcp 连接即可
		dialer, err := net.Dial("tcp", s.address)
		if err != nil {
			log.Println("dialer error:", err)
			return nil, err
		}
		// 建立连接成功
		return dialer, nil
	}
}

// Serve starts the server.
func (s *ReverseClient) Serve() error {
	// 使用 multi_server_conn 注册到多个 grpc server
	mgr := NewConnectionManager(s.tlsconfig)
	wg := sync.WaitGroup{}
	for _, addr := range strings.Split(s.address, ",") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := mgr.Call(context.Background(), addr, tunnelRegister); err != nil {
				log.Println("call error:", err)
				return
			}
		}()
	}
	wg.Wait()
	return nil

	/*
		// Dial the server.
		creds := credentials.NewTLS(s.tlscfg)
		_ = creds
		cc, err := grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			s.address,
			//grpc.WithTransportCredentials(creds),
			// 用 kitex 做 grpcproxy 时不支持客户端证书，gonet 模式启动 kitex 服务可以支持 tls，但是客户端关闭就会 panic
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(s.Dialer()),
		)

		if err != nil {
			return err
		}

		// 注册反向隧道，对 grpc server 端提供服务.
		tunnelStub := tunnelpb.NewTunnelServiceClient(cc)
		channelServer := grpctunnel.NewReverseTunnelServer(tunnelStub)

		// 注册 api
		pb.RegisterRemoteShellServer(channelServer, newRSHServer(s.shell))

		log.Println("Starting Client")
		// Create metadata and context.
		md := metadata.Pairs("client-id", "")
		ctx := metadata.NewOutgoingContext(context.Background(), md)

		// Open the reverse tunnel and serve requests.
		err = retry.Do(
			func() error {
				_, err := channelServer.Serve(ctx)
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(999999),
			retry.Delay(2*time.Second),
			retry.MaxDelay(5*time.Second),
			retry.MaxJitter(3*time.Second),
			retry.DelayType(retry.FixedDelay),
		)
		return err
	*/
}
