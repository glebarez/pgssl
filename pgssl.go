package main

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/jackc/pgproto3/v2"
)

type PgSSL struct {
	pgAddr     string
	clientCert *tls.Certificate
}

func (p *PgSSL) HandleConn(clientConn net.Conn) error {
	// close client connection in the end
	defer clientConn.Close()

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
		_, err := clientConn.Write([]byte{'N'})
		if err != nil {
			return fmt.Errorf("error while sending SSL-decline to client: %s", err)
		}
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
		pgConn.Close()
	}()

	// we pose as a frontend for Postgres connection
	frontend := pgproto3.NewFrontend(pgproto3.NewChunkReader(pgConn), pgConn)

	// send SSL request to postgres
	err = frontend.Send(&pgproto3.SSLRequest{})
	if err != nil {
		return err
	}

	// The server then responds with a single byte containing S or N, indicating that it is willing or unwilling to perform SSL, respectively.
	// If additional bytes are available to read at this point, it likely means that a man-in-the-middle is attempting to perform a buffer-stuffing attack (CVE-2021-23222).
	buf := make([]byte, 2)
	n, err := pgConn.Read(buf)
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("server returned more than 1 byte to SSLrequest, this is not expected")
	}
	if buf[0] == 'N' {
		return fmt.Errorf("server declined SSL communication")
	}
	if buf[0] != 'S' {
		return fmt.Errorf("unexpected response to SSLrequest: %v", buf[0])
	}

	// upgrade connection to TLS
	pgTLSconn := tls.Client(pgConn, &tls.Config{
		GetClientCertificate:     func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) { return p.clientCert, nil },
		InsecureSkipVerify:       true,
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
	})

	// upgrade frontend
	frontend = pgproto3.NewFrontend(pgproto3.NewChunkReader(pgTLSconn), pgTLSconn)
	defer frontend.Send(&pgproto3.Terminate{})

	err = pgTLSconn.Handshake()
	if err != nil {
		return fmt.Errorf("handshake error: %v", err)
	}

	// send original startup
	err = frontend.Send(clientStartupMessage)
	if err != nil {
		return err
	}

	// pipe connections
	pgConnErr, clientConnErr := Pipe(clientConn, pgTLSconn)
	if pgConnErr != nil {
		return fmt.Errorf("postgres connection error: %s", pgConnErr)
	}
	if clientConnErr != nil {
		return fmt.Errorf("client connection error: %s", clientConnErr)
	}

	return nil
}
