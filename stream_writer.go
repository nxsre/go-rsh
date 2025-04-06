package rsh

import (
	"github.com/nxsre/go-rsh/pb"
)

type stdStreamWriter struct {
	stream pb.RemoteShell_SessionServer
}

// Write implements the io.Writer interface
func (s stdStreamWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n > 0 {
		s.stream.Send(&pb.Output{Stdout: p})
	}
	return n, nil
}

type errStreamWriter struct {
	stream pb.RemoteShell_SessionServer
}

// Write implements the io.Writer interface
func (s errStreamWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n > 0 {
		s.stream.Send(&pb.Output{Stderr: p})
	}
	return n, nil
}
