// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package idl

// Creates the .pb.go protobuf definitions.
//go:generate protoc --plugin=../dev-bin/protoc-gen-go --go_out=plugins=grpc:. cli_to_hub.proto hub_to_agent.proto

// Generates mocks for the above definitions.
//go:generate ../dev-bin/mockgen -destination mock_idl/mock_cli_to_hub.pb.go github.com/greenplum-db/gpupgrade/idl CliToHubClient,CliToHubServer,CliToHub_ExecuteServer,CliToHub_ExecuteClient
//go:generate ../dev-bin/mockgen -source hub_to_agent.pb.go -destination mock_idl/mock_hub_to_agent.pb.go
