// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package idl

// Creates the .pb.go protobuf definitions. The use of
// require_unimplemented_servers=false is to use legacy grpc implementation
// that does not care about forward compatibility. We are the sole users and
// maintainers of this grpc definition so this works for us and lets us not
// need to embed a non-functional interface in our hub and agent servers just
// to meet a grpc requirement that does not apply to us.
//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false:. --go-grpc_opt=paths=source_relative common.proto cli_to_hub.proto hub_to_agent.proto

// Generates mocks for the above definitions.
//go:generate ../dev-bin/mockgen -source cli_to_hub_grpc.pb.go -destination mock_idl/mock_cli_to_hub_grpc.pb.go
//go:generate ../dev-bin/mockgen -source hub_to_agent_grpc.pb.go -destination mock_idl/mock_hub_to_agent_grpc.pb.go
