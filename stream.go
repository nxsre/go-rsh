package rsh

import (
	"github.com/nxsre/go-rsh/pb"
	"io"
	"log/slog"
	"os"
	"syscall"
)

func ReadStream(stream pb.RemoteShell_SessionClient, stdout, stderr io.WriteCloser) (*int, error) {
	for {
		select {
		case <-stream.Context().Done():
			slog.Info("Client stream context done")
			// stream Done 之后把结果取回来
			out, err := stream.Recv()
			var exitCode int
			if out != nil {
				stdout.Write(out.Stdout)
				stderr.Write(out.Stderr)
				exitCode = int(out.ExitCode)
			}

			if err == io.EOF {
				slog.Info("Server returned EOF")
				return nil, nil
			}

			if err != nil {
				slog.Info("Error reading stream:", slog.Any("error", err))
				return nil, err
			}
			return &exitCode, nil
		default:
			out, err := stream.Recv()
			if err == io.EOF {
				slog.Info("Server returned EOF 222")
				return nil, nil
			}
			if err != nil {
				slog.Info("Server returned err:: ", slog.Any("err", err))
				return nil, err
			}

			if _, err := stdout.Write(out.Stdout); err != nil {
				return nil, err
			}

			if _, err := stderr.Write(out.Stderr); err != nil {
				return nil, err
			}
			if out.Exited {
				var exitCode int = int(out.ExitCode)
				return &exitCode, err
			}
		}
	}
}

func WriteStream(stream pb.RemoteShell_SessionClient, inc <-chan rune, sigc <-chan os.Signal) {
	for {
		select {
		case <-stream.Context().Done():
			return

		case r := <-inc:
			stream.Send(&pb.Input{Bytes: []byte{byte(r)}})

		case sig := <-sigc:
			if sig == nil {
				continue
			}

			s, ok := sig.(syscall.Signal)
			if !ok {
				slog.Info("Error forwarding signal: os.Signal is not syscall.Signal, signal:", slog.Any("signal", sig))
				break
			}

			switch s {
			default:
				stream.Send(&pb.Input{Signal: int32(s)})
			}
		}
	}
}
