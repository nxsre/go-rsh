package rsh

import (
	"context"
	"fmt"
	"github.com/ibice/go-rsh/pb"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type session struct {
	stream         pb.RemoteShell_SessionServer
	defaultCommand string
	defaultArgs    []string

	cmd  *exec.Cmd
	ptmx *os.File
	lock sync.Mutex

	terminal  bool // 当前 session 是否打开终端
	cmdExitC  chan int
	errC      chan error
	streamInC chan *pb.Input
}

func newSession(stream pb.RemoteShell_SessionServer, defaultCommand string, defaultArgs []string) *session {
	return &session{
		stream:         stream,
		defaultCommand: defaultCommand,
		defaultArgs:    defaultArgs,
		cmdExitC:       make(chan int),
		errC:           make(chan error),
		streamInC:      make(chan *pb.Input),
	}
}

func (s *session) start() error {

	go s.consumeStream()

	for {
		select {

		case <-s.stream.Context().Done():
			log.Println("DEBUG", "stream context done")
			return nil

		case exitCode := <-s.cmdExitC:
			log.Println("DEBUG", "Command exited with code", exitCode)

			if s.terminal {
				// Gracefully close pty to send all output before exiting.
				io.Copy(streamWriter{s.stream}, s.ptmx)
				s.ptmx.Close()
			}

			s.stream.Send(&pb.Output{ExitCode: int32(exitCode), Status: 1})
			return nil

		case err := <-s.errC:
			return err

		case in := <-s.streamInC:
			log.Printf("DEBUG xx22xx, %v %v cmd: %s, %v", in.Terminal, in.Start, in.Command, in.Args)

			if in.Start {
				s.terminal = in.Terminal
				log.Println("DDDDDD", s.terminal, in.Terminal)
				if s.terminal {
					log.Println("DEBUG", "shell session use terminal")
					if err := s.startCommand(s.stream.Context(), in.Command, in.Args); err != nil {
						return fmt.Errorf("start command: %v", err)
					}

					defer s.ptmx.Close()

					go s.notifyOnProcessExit()

					go io.Copy(streamWriter{s.stream}, s.ptmx)
					continue
				} else {
					// 不需要终端时直接执行命令
					log.Printf("DEBUG shell session no terminal, cmd: %s, %v", in.Command, in.Args)
					s.cmd = exec.CommandContext(s.stream.Context(), in.Command, in.Args...)
					stream := streamWriter{s.stream}
					s.cmd.Stdout = stream
					s.cmd.Stderr = stream

					if err := s.cmd.Start(); err != nil {
						if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
							// 命令本身的错误不返回 error，通过 output 传递
							return s.stream.Send(&pb.Output{ExitCode: 127, Status: 1, Bytes: []byte(ee.Error())})
						}
						return err
					}
					go s.notifyOnProcessExit()
					continue
				}
			}

			if err := s.processInput(in); err != nil {
				return fmt.Errorf("processing input: %v", err)
			}
		}
	}
}

func (s *session) startCommand(ctx context.Context, command string, args []string) (err error) {
	if s.cmd != nil {
		return fmt.Errorf("command already running")
	}

	if command == "" {
		command = s.defaultCommand
		args = s.defaultArgs
	}

	log.Println("DEBUG", "Starting command", command, args)

	s.cmd = exec.CommandContext(ctx, command, args...)

	s.ptmx, err = pty.Start(s.cmd)
	if err != nil {
		return fmt.Errorf("start pty: %v", err)
	}

	log.Println("DEBUG", s.cmd.Process, s.cmd.ProcessState)

	return nil
}

func (s *session) processInput(in *pb.Input) error {
	if s.ptmx == nil {
		return fmt.Errorf("received input before the process was started")
	}

	// Handle signals
	if in.Signal != 0 {
		sig := syscall.Signal(in.Signal)

		switch sig {
		case syscall.SIGWINCH:
			sizeParts := strings.Split(string(in.Bytes), " ")
			size := &pty.Winsize{
				Cols: parseUint16(sizeParts[0]),
				Rows: parseUint16(sizeParts[1]),
				X:    parseUint16(sizeParts[2]),
				Y:    parseUint16(sizeParts[3]),
			}

			log.Println("DEBUG", "Setting window size to", size)

			if err := pty.Setsize(s.ptmx, size); err != nil {
				return fmt.Errorf("setsize: %v", err)
			}

		default:
			if s.cmd.Process == nil {
				return fmt.Errorf("tried to signal nil process")
			}

			if err := s.cmd.Process.Signal(sig); err != nil {
				return fmt.Errorf("signal: %v", err)
			}
		}

		return nil
	}

	_, err := s.ptmx.Write(in.Bytes)
	if err != nil {
		return fmt.Errorf("write ptmx: %v", err)
	}

	return nil
}

func (s *session) consumeStream() {
	for {
		in, err := s.stream.Recv()
		if err != nil {
			s.errC <- fmt.Errorf("recv: %v", err)
		}
		s.streamInC <- in
	}
}

func (s *session) notifyOnProcessExit() {
	log.Println("DEBUG", "Waiting for process completion")

	if s.cmd.Err != nil {
		s.errC <- fmt.Errorf("cmd err: %v", s.cmd.Err)
		return
	}

	ps, err := s.cmd.Process.Wait()

	log.Println("DEBUG", "Process completed", ps, err)

	if err != nil {
		s.errC <- fmt.Errorf("cmd wait: %v", err)
		return
	}

	s.cmdExitC <- ps.ExitCode()
}
