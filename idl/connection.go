// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package idl

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

type Connection struct {
	Conn          *grpc.ClientConn
	AgentClient   AgentClient
	Hostname      string
	CancelContext func()
}

// ServerAlreadyStopped checks the gRPC error to determine if the server is
// already stopped or not. When stopping a gRPC server that is already down we
// do not  want to return an error since it is already stopped. However, there
// is no clean way to differentiate based on the gRPC error code if a server is
// already stopped versus generally unavailable. Thus, ignore the gRPC error
// returned when trying to access an already stopped gRPC server.
// See https://github.com/grpc/grpc/blob/v1.56.2/doc/statuscodes.md
func ServerAlreadyStopped(err error) bool {
	errStatus := grpcStatus.Convert(err)
	if errStatus.Code() == codes.Unavailable && errStatus.Message() == "error reading from server: EOF" {
		return true
	}

	return false
}
