package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
)

var options struct {
	listenAddress string
	pgAddress     string
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:  %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	// parse arguments
	flag.StringVar(&options.listenAddress, "l", "127.0.0.1:15432", "Listen address")
	flag.StringVar(&options.pgAddress, "p", "", "Postgres address")
	flag.Parse()

	if options.pgAddress == "" {
		log.Fatal("postgres address must be specified")
	}

	// bind listening socket
	ln, err := net.Listen("tcp", options.listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening on", ln.Addr())

	// create pgssl instance
	pgSSL := NewPgSSL(options.pgAddress)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Accepted connection from", conn.RemoteAddr())

		go func() {
			err := pgSSL.HandleConn(conn)
			if err != nil {
				log.Println(err)
			}
			log.Println("Closed connection from", conn.RemoteAddr())
		}()
	}
}
