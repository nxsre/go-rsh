// This program is used to test the SIGCHLD signal.
//
// It runs a child process and waits for it to exit. When the child process
// exits, it sends a SIGCHLD signal to the parent process. The parent process
// then waits for the SIGCHLD signal and prints a message. The parent should
// receive the SIGCHLD signal only once.
//
// Go 1.23.6 seems to have a bug where it receives the SIGCHLD signal twice.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

var (
	isChild bool
)

func init() {
	flag.BoolVar(&isChild, "child", false, "if true, run as a child process")
}

func child() {
	fmt.Println("I am a child process")

	time.Sleep(20 * time.Second)

	fmt.Println("Child process exiting")

	os.Exit(0)
}

func main() {
	flag.Parse()

	if isChild {
		child()
		return
	}

	fmt.Println("I am a parent process")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	// Start the child process.
	cmd := exec.Command(os.Args[0], "-child")

	go func() {
		numSigchld := 0
		for {
			sig := <-sigChan
			numSigchld++
			fmt.Printf("Received %d %v\n", numSigchld, sig)
			cmd.Process.Signal(sig)
		}
	}()

	err := cmd.Start()
	if err != nil {
		panic(err)
	}

	// Wait for the child process to exit.
	cmd.Wait()

	// Wait for any SIGCHLD signals.
	time.Sleep(1 * time.Second)

	os.Exit(0)
}
