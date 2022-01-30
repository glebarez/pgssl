package main

import (
	"fmt"
	"log"
	"net"

	"github.com/jackc/pgproto3/v2"
)

type PgSSL struct {
	pgAddr string
}

func NewPgSSL(pgAddr string) *PgSSL {
	return &PgSSL{pgAddr: pgAddr}
}

func (p *PgSSL) HandleConn(clientConn net.Conn) error {
	// close client connection in the end
	defer func() {
		log.Print("closing client connection")
		clientConn.Close()
	}()

	// we pose as a backend for client connection
	backend := pgproto3.NewBackend(pgproto3.NewChunkReader(clientConn), clientConn)

	// receive startup message from the client
	clientStartupMessage, err := backend.ReceiveStartupMessage()
	if err != nil {
		return fmt.Errorf("error receiving startup message: %w", err)
	}

	switch clientStartupMessage.(type) {
	case *pgproto3.StartupMessage:
		// ok
	case *pgproto3.SSLRequest:
		return fmt.Errorf("client must not use SSL")
	default:
		return fmt.Errorf("unknown startup message: %#v", clientStartupMessage)
	}

	// open connection to postgres backend
	pgConn, err := net.Dial("tcp", p.pgAddr)
	if err != nil {
		return err
	}
	defer func() {
		log.Print("closing postgres connection")
		pgConn.Close()
	}()

	// we pose as a frontend for Postgres connection
	frontend := pgproto3.NewFrontend(pgproto3.NewChunkReader(pgConn), pgConn)

	// forward startup to the postgres
	err = frontend.Send(clientStartupMessage)
	if err != nil {
		return err
	}

	// pipe connections
	Pipe(pgConn, clientConn)

	return nil
}

// chanFromConn creates a channel from a Conn object, and sends everything it
// Read()s from the socket to the channel.
func chanFromConn(conn net.Conn) chan []byte {
	c := make(chan []byte)

	go func() {
		b := make([]byte, 1024)

		for {
			n, err := conn.Read(b)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, b[:n])
				c <- res
			}
			if err != nil {
				log.Println(err)
				c <- nil
				break
			}
		}
	}()

	return c
}

// Pipe creates a full-duplex pipe between the two sockets and transfers data from one to the other.
func Pipe(conn1 net.Conn, conn2 net.Conn) {
	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)

	for {
		select {
		case b1 := <-chan1:
			if b1 == nil {
				return
			} else {
				conn2.Write(b1)
			}
		case b2 := <-chan2:
			if b2 == nil {
				return
			} else {
				conn1.Write(b2)
			}
		}
	}
}
