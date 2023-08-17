package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
)

var options struct {
	listenAddress  string
	pgAddress      string
	clientCertPath string
	clientKeyPath  string
}

func argFatal(s string) {
	fmt.Fprintln(os.Stderr, s)
	flag.Usage()
	os.Exit(1)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:  %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	// parse arguments
	flag.StringVar(&options.listenAddress, "l", "127.0.0.1:15432", "Listen address")
	flag.StringVar(&options.pgAddress, "p", "", "Postgres address")
	flag.StringVar(&options.clientCertPath, "c", "", "clientCertPath")
	flag.StringVar(&options.clientKeyPath, "k", "", "clientKeyPath")
	flag.Parse()

	if options.pgAddress == "" {
		argFatal("postgres address must be specified")
	}
	if (options.clientCertPath == "") != (options.clientKeyPath == "") {
		argFatal("You must specify both clientKeyPath and clientCertPath to use a client certificate")
	}

	// load client certificate and key
	cert, err := tls.LoadX509KeyPair(options.clientCertPath, options.clientKeyPath)
	if err != nil {
		log.Fatal(err)
	}

	// create pgSSL instance
	pgSSL := &PgSSL{
		pgAddr:     options.pgAddress,
		clientCert: &cert,
	}

	// bind listening socket
	ln, err := net.Listen("tcp", options.listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening on", ln.Addr())

	// start accepting connection
	var connNum int
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		connNum++
		log.Printf("[%3d] Accepted connection from %s\n", connNum, conn.RemoteAddr())

		// handle connection in goroutine
		go func(n int) {
			err := pgSSL.HandleConn(conn)
			if err != nil {
				log.Printf("[%3d] error in connection: %s", n, err)
			}
			log.Printf("[%3d] Closed connection from %s", n, conn.RemoteAddr())
		}(connNum)
	}
}
