package rsh

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	"github.com/kos-v/dsnparser"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"net"
	"os"
	"strings"
	"sync"
)

// Example Usage
/*func fn() {
	ctx := context.Background()

	addrs := []string{"hostA:20023", "hostB:20023", "hostC:20023"}
	mgr := NewConnectionManager()
	for _, addr := range addrs {
		if err := mgr.Call(ctx, addr, tunnelRegister); err != nil {
			// handle
		}
	}
}*/

// 获取唯一ID
func GetNodeID() string {
	if os.Getenv("NODE_ID") != "" {
		return os.Getenv("NODE_ID")
	}
	hostname, err := os.Hostname()
	if err == nil {
		return hostname
	}

	id, err := machineid.ID()
	if err == nil {
		return id
	}

	return uuid.NewString()
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
	tlscfg *tls.Config
}

func NewConnectionManager(tlscfg *tls.Config) *ConnectionManager {
	return &ConnectionManager{conns: map[string]*Connection{}, tlscfg: tlscfg}
}

func (m *ConnectionManager) Call(ctx context.Context, address string, fn CallFn) error {
	conn, err := m.Connect(ctx, address)
	if err != nil {
		return err
	}

	// You could have exponential retry / backoff up to N times
	return fn(context.Background(), conn)
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
	// 判断是否使用 tls
	dsn := dsnparser.Parse(address)
	var (
		cc  *grpc.ClientConn
		err error
	)

	switch dsn.GetScheme() {
	case "http", "tcp", "":
		cc, err = grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			fmt.Sprintf("%s:%s", dsn.GetHost(), dsn.GetPort()),
			// 用 kitex 做 grpcproxy 时不支持客户端证书，gonet 模式启动 kitex 服务可以支持 tls，但是客户端关闭就会 panic
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	case "tls", "https":
		creds := credentials.NewTLS(m.tlscfg)

		cc, err = grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			fmt.Sprintf("%s:%s", dsn.GetHost(), dsn.GetPort()),
			grpc.WithTransportCredentials(creds),
		)
	case "unix":
		creds := insecure.NewCredentials()
		if m.tlscfg != nil {
			creds = credentials.NewTLS(m.tlscfg)
		}

		address = strings.TrimPrefix(address, `unix://`)
		cc, err = grpc.NewClient(
			// 协议最好使用passthrough，要不然默认的使用的是 unix
			fmt.Sprintf("%s", address),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return net.Dial("unix", address)
			}),
			grpc.WithResolvers(&builder{}),
			grpc.WithTransportCredentials(creds),
			grpc.WithAuthority(m.tlscfg.ServerName),
		)
	}

	return cc, err
}

type builder struct{}

func (*builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	cc.UpdateState(resolver.State{Addresses: []resolver.Address{{Addr: target.Endpoint()}}})
	return nil, nil
}

func (*builder) Scheme() string {
	return ""
}

var _ resolver.Builder = (*builder)(nil)

func (m *ConnectionManager) Close() {
	// Close all connections
	m.mu.Lock()
	for _, conn := range m.conns {
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
