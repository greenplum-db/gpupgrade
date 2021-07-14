//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package idl

import (
	"google.golang.org/grpc"
)

type Connection struct {
	Conn          *grpc.ClientConn
	AgentClient   AgentClient
	Hostname      string
	CancelContext func()
}
