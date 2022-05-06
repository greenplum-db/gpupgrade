// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

// +build tools

// The tools pseudo-package is used to explicitly record Go tool dependencies in
// a module-aware world. It replaces the dep "required" flow. Tools declared
// here can be installed into dev-bin/ using the depend-dev recipe in the
// top-level Makefile.
//
// See
//
//    https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
//
// for information.
package tools

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/protoc-gen-go"
)
