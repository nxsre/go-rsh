package rsh

import (
	"context"
	"fmt"
	"github.com/asaskevich/govalidator"
	"github.com/avast/retry-go"
	"github.com/ibice/go-rsh/pb"
	"github.com/jhump/grpctunnel"
	"github.com/jhump/grpctunnel/tunnelpb"
	"github.com/kos-v/dsnparser"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"log"
	"sync"
	"time"
)

// Example Usage
func xx() {
	//ctx := context.Background()
	//
	//addrs := []string{"hostA:20023", "hostB:20023", "hostC:20023"}
	//mgr := NewConnectionManager()
	//for _, addr := range addrs {
	//	if err := mgr.Call(ctx, addr, doThing); err != nil {
	//		// handle
	//	}
	//}
}

func doThing(ctx context.Context, conn *Connection) error {
	// 注册反向隧道，对 grpc server 端提供服务.
	tunnelStub := tunnelpb.NewTunnelServiceClient(conn)
	channelServer := grpctunnel.NewReverseTunnelServer(tunnelStub)

	// 注册 api
	pb.RegisterRemoteShellServer(channelServer, newRSHServer("/bin/sh"))

	log.Println("Starting Client")
	// Create metadata and context.
	md := metadata.Pairs("client-id", "big-niubi")
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
		retry.Attempts(999999),
		retry.Delay(2*time.Second),
		retry.MaxDelay(5*time.Second),
		retry.MaxJitter(3*time.Second),
		retry.DelayType(retry.FixedDelay),
	)
	return err
}

// Types

type CallFn func(context.Context, *Connection) error

type Connection struct {
	once    sync.Once
	manager *ConnectionManager
	*grpc.ClientConn
}

func (c *Connection) Close() error {
	var err error
	c.once.Do(func() {
		// Remove from the source manager on close
		err = c.manager.CloseConnection(c.ClientConn.Target())
	})
	return err
}

type ConnectionManager struct {
	mu     sync.RWMutex
	conns  map[string]*Connection
	client *ReverseClient
}

func NewConnectionManager(client *ReverseClient) *ConnectionManager {
	return &ConnectionManager{conns: map[string]*Connection{}, client: client}
}

func (m *ConnectionManager) Call(ctx context.Context, address string, fn CallFn) error {
	conn, err := m.Connect(ctx, address)
	if err != nil {
		return err
	}

	// You could have exponential retry / backoff up to N times
	go fn(context.Background(), conn)
	return nil
}

func (m *ConnectionManager) Connect(ctx context.Context, address string) (*Connection, error) {
	conn := m.conns[address]
	if conn != nil {
		return conn, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check if we have a connection
	if conn := m.conns[address]; conn != nil {
		return conn, nil
	}

	// Create a new underlying client connection
	cc, err := m.newConnection(ctx, address)
	if err != nil {
		return nil, err
	}

	// Wrap the underlying connection
	conn = &Connection{
		ClientConn: cc,
		manager:    m,
	}
	m.conns[address] = conn

	return conn, nil
}

func (m *ConnectionManager) newConnection(ctx context.Context, address string) (*grpc.ClientConn, error) {
	creds := credentials.NewTLS(m.client.tlsconfig)
	_ = creds

	// 判断是否使用 tls
	dsn := dsnparser.Parse(address)
	var (
		cc  *grpc.ClientConn
		err error
	)
	log.Println("地址是否有效", dsn.GetScheme(), address, govalidator.IsURL(address))

	switch dsn.GetScheme() {
	case "http", "tcp", "":
		cc, err = grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			fmt.Sprintf("%s:%s", dsn.GetHost(), dsn.GetPort()),
			// 用 kitex 做 grpcproxy 时不支持客户端证书，gonet 模式启动 kitex 服务可以支持 tls，但是客户端关闭就会 panic
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			//grpc.WithContextDialer(m.server.Dialer()),
		)
	case "tls", "https":
		cc, err = grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			fmt.Sprintf("%s:%s", dsn.GetHost(), dsn.GetPort()),
			grpc.WithTransportCredentials(creds),
			//grpc.WithContextDialer(m.server.Dialer()),
		)
		log.Println(cc, err)
	}

	return cc, err
}

func (m *ConnectionManager) Close() {
	// Close all connections
	m.mu.Lock()
	for address, conn := range m.conns {
		log.Println(address)
		if err := conn.ClientConn.Close(); err != nil {
			// Log / handle
		}
	}
	m.conns = map[string]*Connection{}
	m.mu.Unlock()

}

func (m *ConnectionManager) CloseConnection(address string) error {
	return m.closeConnection(address)
}

func (m *ConnectionManager) closeConnection(address string) (err error) {
	m.mu.Lock()
	if conn, ok := m.conns[address]; ok {
		err = conn.ClientConn.Close()
		delete(m.conns, address)
	}
	m.mu.Unlock()
	return
}
