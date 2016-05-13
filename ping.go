package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"fmt"
	"strconv"

	"github.com/Scalingo/go-graceful-restart-example/logger"
	"github.com/Scalingo/go-graceful-restart-example/server"
)

func getConnFDs(startPos int) (ConnFDs []uintptr) {
	connCountStr := os.Getenv("_GRACEFUL_RESTART_CONN_COUNT")
	connCount, err := strconv.Atoi(connCountStr)
	if err != nil {
		fmt.Printf("Fail to restore the conn fds")
	}
	for i := startPos + 1; i <= startPos+connCount; i++ {
		ConnFDs = append(ConnFDs, uintptr(i))
	}
	return
}

func main() {
	log := logger.New("Server")

	var s *server.Server
	var err error
	if os.Getenv("_GRACEFUL_RESTART") == "true" {
		connFDs := getConnFDs(3)
		log.Printf("get connFDs %v", connFDs)
		//the second parameter is 0 is because in execSpec.Files, ListenerFD's index is 0
		s, err = server.NewFromFD(log, 3, connFDs)

	} else {
		s, err = server.New(log, 12345)
	}
	if err != nil {
		log.Fatalln("fail to init server:", err)
	}
	log.Println("Listen on", s.Addr())

	go s.StartAcceptLoop()

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM)
	for sig := range signals {
		if sig == syscall.SIGTERM {
			// Stop accepting new connections
			s.Stop()
			// Wait a maximum of 10 seconds for existing connections to finish
			err := s.WaitWithTimeout(10 * time.Second)
			if err == server.WaitTimeoutError {
				log.Printf("Timeout when stopping server, %d active connections will be cut.\n", s.ConnectionsCounter())
				os.Exit(-127)
			}
			// Then the program exists
			log.Println("Server shutdown successful")
			os.Exit(0)
		} else if sig == syscall.SIGHUP {
			// Stop accepting requests
			s.Stop()
			// Get socket file descriptor to pass it to fork
			listenerFD, err := s.ListenerFD()
			if err != nil {
				log.Fatalln("Fail to get socket file descriptor:", err)
			}
			// Set a flag for the new process start process
			os.Setenv("_GRACEFUL_RESTART", "true")
			os.Setenv("_GRACEFUL_RESTART_CONN_COUNT", strconv.Itoa(len(s.CM.Conns)))
			execSpec := &syscall.ProcAttr{
				Env:   os.Environ(),
				Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(), listenerFD},
			}
			//Passing all alive sockets to the child process
			for _, tcpConn := range s.CM.Conns {
				file, _ := tcpConn.File()
				execSpec.Files = append(execSpec.Files, file.Fd())
			}
			// Fork exec the new version of your server
			log.Println("execSpec", execSpec)
			//ForkExec will go fork a child and keep the fd open.
			//Find and exec the binary in the current location.
			//Wiki for fork-exec
			//https://en.wikipedia.org/wiki/Fork%E2%80%93exec

			fork, err := syscall.ForkExec(os.Args[0], os.Args, execSpec)
			if err != nil {
				log.Fatalln("Fail to fork", err, listenerFD)
			}
			log.Println("SIGHUP received: fork-exec to", fork)
			// Wait for all conections to be finished
			<-time.After(2 * time.Second)
			log.Println(os.Getpid(), "Server gracefully shutdown")

			// Stop the old server, all the connections have been closed and the new one is running
			os.Exit(0)
		}
	}
}
