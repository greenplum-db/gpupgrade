// The port_listener utility listens on the specified port on all ipv4 and ipv6
// interfaces. If no ipv6 interface is detected it will not listen on ipv6.
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
	"os/signal"
	"syscall"
)

func main() {
	args := os.Args[1:]
	if len(args) != 1 {
		log.Fatalf("usage: %s <port_number>", os.Args[0])
	}

	port := args[0]

	signals := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	log := "tcp4"

	go func() {
		connect("tcp4", port)
	}()

	if isIpv6Enabled() {
		log += " and tcp6"
		go func() {
			connect("tcp6", port)
		}()
	}

	fmt.Printf("listening on %s on port %s...\n", log, port)

	go func() {
		signal := <-signals
		fmt.Printf("\nReceived %s. Exiting.", signal)
		done <- true
	}()

	<-done
}

func connect(network string, port string) net.Listener {
	listener, err := net.Listen(network, ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on port %s for %s: %v", port, network, err)
	}

	_, err = listener.Accept()
	if err != nil {
		log.Fatalf("failed to accept on port %s for %s: %v", port, network, err)
	}

	return listener
}

func isIpv6Enabled() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("failed to determine interface addrs: %v", err)
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			log.Fatalf("failed to parse network address %s: %v", addr.String(), err)
		}

		if ip.To4() == nil {
			return true
		}
	}

	return false
}
