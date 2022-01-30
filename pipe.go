package main

import (
	"io"
	"net"
)

type readResult struct {
	data []byte
	err  error
}

// chanFromConn creates a channel from a Conn object, and sends everything it
// Read()s from the socket to the channel.
// channel delivers {[]byte, nil} after successfull read.
// channel delivers {nil,err} in case of error.
// channel is closed when EOF is received.
func chanFromConn(conn net.Conn) chan readResult {
	c := make(chan readResult)

	go func() {
		// make buffer to receive data
		buf := make([]byte, 1024)

		for {
			n, err := conn.Read(buf)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, buf[:n])
				c <- readResult{res, nil}
			}
			if err == io.EOF {
				close(c)
				return
			}
			if err != nil {
				c <- readResult{nil, err}
				break
			}
		}
	}()

	return c
}

// Pipe creates a full-duplex pipe between the two sockets and transfers data from one to the other.
func Pipe(conn1 net.Conn, conn2 net.Conn) (e1, e2 error) {
	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)

	for {
		select {
		case b1, ok := <-chan1:
			if !ok {
				return // connection was closed
			}
			if b1.err != nil {
				e1 = b1.err
				return
			} else {
				conn2.Write(b1.data)
			}
		case b2, ok := <-chan2:
			if !ok {
				return // connection was closed
			}
			if b2.err != nil {
				e2 = b2.err
				return
			} else {
				conn1.Write(b2.data)
			}
		}
	}
}
