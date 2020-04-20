// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

// The port_listener utility listens on the specified port even during gRPC
// retries. One can inspect the ports in use on OS X with `netstat -an` rather
// than `lsof -Pi`.
// Usage:
//
//   go run port_listener <port>
//
package main

import (
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) != 1 {
		log.Fatalf("usage: %s <port_number>", os.Args[0])
	}

	port := args[0]

	_, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
	}

	fmt.Printf("listening on port %s...\n", port)

	select {} // wait forever
}
