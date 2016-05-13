package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Scalingo/go-graceful-restart-example/logger"
)

type Server struct {
	CM     *ConnectionManager
	logger *logger.Logger
	socket *net.TCPListener
}

func New(logger *logger.Logger, port int) (*Server, error) {
	s := &Server{CM: NewConnectionManager(), logger: logger}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("fail to resolve addr: %v", err)
	}
	sock, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("fail to listen tcp: %v", err)
	}

	s.socket = sock
	return s, nil
}

func NewFromFD(logger *logger.Logger, fd uintptr, connFDs []uintptr) (*Server, error) {
	s := &Server{CM: NewConnectionManager(), logger: logger}

	//there is no reason except that there is no other way to recover a *os.File from a file descriptor number for a unnamed file (like a socket).
	//So you have to pass an arbitrary name to satisfy the function call.
	file := os.NewFile(fd, "/tmp/sock-go-graceful-restart")
	listener, err := net.FileListener(file)
	if err != nil {
		return nil, errors.New("File to recover socket from file descriptor: " + err.Error())
	}
	listenerTCP, ok := listener.(*net.TCPListener)
	if !ok {
		return nil, fmt.Errorf("File descriptor %d is not a valid TCP socket", fd)
	}
	s.socket = listenerTCP

	for _, connFD := range connFDs {
		logger.Println("about to recover connfd: ", connFD)
		err := recoverConn(logger, connFD)
		if err != nil {
			logger.Println("Failed to recover connFd ", connFD)
		}
	}
	return s, nil
}

func recoverConn(logger *logger.Logger, fd uintptr) error {
	//same as listener fd, the name which is the second parameter of NewFile is required but it means nothing.
	file := os.NewFile(fd, "/tmp/sock-go-graceful-restart")
	conn, err := net.FileConn(file)
	if err != nil {
		return errors.New("File to recover connection from file descriptor: " + err.Error())
	}
	connTCP, ok := conn.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("File descriptor %d is not a valid TCP socket", fd)
	}
	go handleConn(logger, connTCP)
	return nil
}

func (s *Server) Stop() {
	// Accept will instantly return a timeout error
	s.socket.SetDeadline(time.Now())
}

func (s *Server) ListenerFD() (uintptr, error) {
	file, err := s.socket.File()
	if err != nil {
		return 0, err
	}
	return file.Fd(), nil
}

func (s *Server) Wait() {
	s.CM.Wait()
}

var WaitTimeoutError = errors.New("timeout")

func (s *Server) WaitWithTimeout(duration time.Duration) error {
	timeout := time.NewTimer(duration)
	wait := make(chan struct{})
	go func() {
		s.Wait()
		wait <- struct{}{}
	}()

	select {
	case <-timeout.C:
		return WaitTimeoutError
	case <-wait:
		return nil
	}
}

func (s *Server) StartAcceptLoop() {
	for {
		conn, err := s.socket.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				s.logger.Println("Stop accepting connections")
				return
			}
			s.logger.Println("[Error] fail to accept:", err)
		}
		go func() {
			s.CM.Add(1)
			tcpConn, _ := conn.(*net.TCPConn)
			s.CM.Conns = append(s.CM.Conns, tcpConn)
			s.handleConn(conn)
			s.CM.Done()
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	handleConn(s.logger, conn)
}

func handleConn(logger *logger.Logger, conn net.Conn) {
	tick := time.NewTicker(time.Second)
	buffer := make([]byte, 64)
	for {
		select {
		case <-tick.C:
			_, err := conn.Write([]byte("ping"))
			if err != nil {
				logger.Println("[Error] fail to write 'ping':", err)
				conn.Close()
				return
			}
			logger.Println("[Server] Sent 'ping'\n")

			n, err := conn.Read(buffer)
			if err != nil {
				logger.Println("[Error] fail to read from socket:", err)
				conn.Close()
				return
			}

			logger.Printf("[Server][Conn:%s] OK: read %d bytes: '%s'\n", conn.RemoteAddr(), n, string(buffer[:n]))
		}
	}
}

func (s *Server) Addr() net.Addr {
	return s.socket.Addr()
}

func (s *Server) ConnectionsCounter() int {
	return s.CM.Counter
}
