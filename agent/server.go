// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"sync"

	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/logger"
)

type Server struct {
	conf Config

	mu      sync.Mutex
	server  *grpc.Server
	lis     net.Listener
	stopped chan struct{}
	daemon  bool
}

type Config struct {
	Port     int
	StateDir string
}

func NewServer(conf Config) *Server {
	return &Server{
		conf:    conf,
		stopped: make(chan struct{}, 1),
	}
}

// MakeDaemon tells the Server to disconnect its stdout/stderr streams after
// successfully starting up.
func (s *Server) MakeDaemon() {
	s.daemon = true
}

func (s *Server) Start() {
	err := createStateDirectory(s.conf.StateDir)
	if err != nil {
		log.Fatalf("failed to create state directory: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(s.conf.Port))
	if err != nil {
		// FIXME: This should be log.Fatal which returns exit code 1. However,
		//   with the --daemonize flag it returns exit code 0 indicating no
		//   error to the caller. Thus, use log.Panic to return exit code 2 to
		//   indicate an error.
		log.Panicf("failed to listen: %v", err)
	}

	// Set up an interceptor function to log any panics we get from request
	// handlers.
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer logger.WritePanics()
		return handler(ctx, req)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

	s.mu.Lock()
	s.server = server
	s.lis = lis
	s.mu.Unlock()

	idl.RegisterAgentServer(server, s)
	reflection.Register(server)

	if s.daemon {
		log.Printf("Agent started on port %d with pid %d", s.conf.Port, os.Getpid())
		daemon.Daemonize()
	}

	err = server.Serve(lis)
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	s.stopped <- struct{}{}
}

func (s *Server) StopAgent(ctx context.Context, in *idl.StopAgentRequest) (*idl.StopAgentReply, error) {
	s.Stop()
	return &idl.StopAgentReply{}, nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		s.server.Stop()
		<-s.stopped
	}
}

func createStateDirectory(dir string) error {
	// When the agent is started it is passed the state directory. Ensure it also
	// sets GPUPGRADE_HOME in its environment such that utils functions work.
	// This is critical for our acceptance tests which often set GPUPGRADE_HOME.
	err := os.Setenv("GPUPGRADE_HOME", dir)
	if err != nil {
		return xerrors.Errorf("set GPUPGRADE_HOME=%s: %w", dir, err)
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return xerrors.Errorf("create state directory %q: %w", dir, err)
	}

	return nil
}
