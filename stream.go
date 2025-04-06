package rsh

import (
	"github.com/ibice/go-rsh/pb"
	"io"
	"log"
	"os"
	"syscall"
)

func ReadStream(stream pb.RemoteShell_SessionClient, outer io.WriteCloser) (*int, error) {
	for {
		select {
		case <-stream.Context().Done():
			log.Println("Client stream context done")
			// stream Done 之后把结果取回来
			out, err := stream.Recv()
			var exitCode int
			if out != nil {
				outer.Write(out.Bytes)
				exitCode = int(out.ExitCode)
			}

			if err == io.EOF {
				log.Print("Server returned EOF")
				return nil, nil
			}

			if err != nil {
				log.Println("Error reading stream:", err)
				return nil, err
			}
			return &exitCode, nil
		default:
			out, err := stream.Recv()
			if err == io.EOF {
				log.Print("Server returned EOF 222")
				return nil, nil
			}
			if err != nil {
				log.Print("Server returned err:: ", err)
				return nil, err
			}

			outer.Write(out.Bytes)
			if out.Status == 1 {
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
				log.Println("Error forwarding signal: os.Signal is not syscall.Signal, signal:", sig.String())
				break
			}

			switch s {
			default:
				stream.Send(&pb.Input{Signal: int32(s)})
			}
		}
	}
}
