// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
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
	mutex       sync.Mutex
	gRPCserver  *grpc.Server
	listener    net.Listener
	stoppedChan chan struct{}
}

func New() *Server {
	return &Server{
		stoppedChan: make(chan struct{}, 1),
	}
}

func (s *Server) Start(port int, stateDir string, daemonize bool) error {
	err := createStateDirectory(stateDir)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", port, err)
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer logger.WritePanics()
		return handler(ctx, req)
	}
	gRPCserver := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

	s.mutex.Lock()
	s.gRPCserver = gRPCserver
	s.listener = listener
	s.mutex.Unlock()

	idl.RegisterAgentServer(gRPCserver, s)
	reflection.Register(gRPCserver)

	if daemonize {
		log.Printf("Agent started on port %d with pid %d", port, os.Getpid())
		daemon.Daemonize()
	}

	err = gRPCserver.Serve(listener)
	if err != nil {
		return fmt.Errorf("agent gRPC Serve: %w", err)
	}

	s.stoppedChan <- struct{}{}
	return nil
}

func (s *Server) StopAgent(ctx context.Context, in *idl.StopAgentRequest) (*idl.StopAgentReply, error) {
	s.Stop()
	return &idl.StopAgentReply{}, nil
}

func (s *Server) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.gRPCserver != nil {
		s.gRPCserver.Stop()
		<-s.stoppedChan
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
