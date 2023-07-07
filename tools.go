// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

//go:build tools

// The tools pseudo-package is used to explicitly record Go tool dependencies in
// a module-aware world. It replaces the dep "required" flow. Tools declared
// here can be installed into dev-bin/ using the depend-dev recipe in the
// top-level Makefile.
//
// For more information see:
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import (
	_ "github.com/golang/mock/mockgen"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
)
