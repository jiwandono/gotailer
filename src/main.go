package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: gotailer <ADDRESS:PORT> <WORKING DIR> <COMMAND> [ARGUMENTS]")
		return
	}

	err := run()
	if err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}

func run() error {
	cmd := exec.Command(os.Args[3], os.Args[4:]...)
	cmd.Dir = os.Args[2]

	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return errors.New("error creating stdout pipe: " + err.Error())
	}

	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		return errors.New("error creating stderr pipe: " + err.Error())
	}

	listen, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		return errors.New("error creating listening socket: " + err.Error())
	}
	fmt.Printf("listening on %v\n", listen.Addr())

	gotailer := newGotailerServer()
	httpServer := &http.Server{
		Handler:      gotailer,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
	}

	// Stdout scanner goroutine
	go func() {
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			gotailer.publish(scanner.Bytes())
		}
	}()

	// Stderr scanner goroutine
	go func() {
		scanner := bufio.NewScanner(stderrReader)
		for scanner.Scan() {
			gotailer.publish(scanner.Bytes())
		}
	}()

	fmt.Printf("starting command...\n")
	err = cmd.Start()
	if err != nil {
		return errors.New("error starting command: " + err.Error())
	}
	fmt.Printf("command started successfully\n")

	// Termination channel: HTTP server error
	httpErrorChan := make(chan error, 1)
	go func() {
		httpErrorChan <- httpServer.Serve(listen)
	}()

	// Termination channel: OS-level signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Termination channel: command execution state
	waitChan := make(chan interface{}, 1)
	go func() {
		waitErr := cmd.Wait()
		if waitErr == nil {
			waitChan <- "without error"
		} else {
			waitChan <- waitErr
		}
	}()

	select {
	case err := <-httpErrorChan:
		fmt.Printf("http server error: %v\n", err)
	case sig := <-signalChan:
		fmt.Printf("signal received: %v\n", sig)
	case wait := <-waitChan:
		fmt.Printf("command exited: %v\n", wait)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	return httpServer.Shutdown(ctx)
}
