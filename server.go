package rsh

import (
	"fmt"
	"github.com/nxsre/go-rsh/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log/slog"
	"net"
	"os/exec"
)

// Server is the remote shell server.
type Server struct {
	address string
	shell   string
}

// NewServer creates a new remote shell server.
func NewServer(address string, shell string) *Server {
	return &Server{
		address: address,
		shell:   shell,
	}
}

// Serve starts the server.
func (s *Server) Serve() error {
	l, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen: %v", err)
	}

	g := grpc.NewServer()

	pb.RegisterRemoteShellServer(g, newRSHServer(s.shell))

	reflection.Register(g)

	return g.Serve(l)
}

type rshServer struct {
	pb.UnimplementedRemoteShellServer
	shell string
}

func newRSHServer(shell string) *rshServer {
	return &rshServer{shell: shell}
}

func (s *rshServer) Session(stream pb.RemoteShell_SessionServer) error {
	slog.Info("Opening session")
	sess := newSession(stream, s.shell, nil)
	if err := sess.start(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			_ = exitErr
		} else {
			slog.Info("执行命令出错", slog.Any("err", err))
		}
		return err
	}
	slog.Info("Session closed")
	return nil
}
