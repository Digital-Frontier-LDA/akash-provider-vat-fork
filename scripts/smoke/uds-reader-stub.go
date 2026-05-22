// Command uds-reader-stub is a minimal stand-in for the Phase v1.0-03 sidecar.
//
// It listens on a Unix domain socket and appends every line-delimited NDJSON
// record it receives to an output file. It keeps accepting connections so the
// provider can reconnect. Build it separately from the df-telemetry package:
//
//	go build -o uds-reader-stub scripts/smoke/uds-reader-stub.go
//
// Env:
//	DF_TELEMETRY_SOCKET    UDS path to listen on (required)
//	DF_SMOKE_NDJSON_OUT    file to append received NDJSON lines to (required)
package main

import (
	"bufio"
	"log"
	"net"
	"os"
)

func main() {
	sock := os.Getenv("DF_TELEMETRY_SOCKET")
	out := os.Getenv("DF_SMOKE_NDJSON_OUT")
	if sock == "" || out == "" {
		log.Fatal("uds-reader-stub: DF_TELEMETRY_SOCKET and DF_SMOKE_NDJSON_OUT must be set")
	}

	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		log.Fatalf("uds-reader-stub: listen %s: %v", sock, err)
	}
	defer ln.Close()
	log.Printf("uds-reader-stub: listening on %s, appending NDJSON to %s", sock, out)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("uds-reader-stub: accept: %v", err)
			continue
		}
		go handle(conn, out)
	}
}

func handle(conn net.Conn, out string) {
	defer conn.Close()
	f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("uds-reader-stub: open %s: %v", out, err)
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		if _, err := f.Write(append(sc.Bytes(), '\n')); err != nil {
			log.Printf("uds-reader-stub: write: %v", err)
			return
		}
	}
}
