//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package versioner

// Obtain is an interface for obtaining greenplum and gpupgrade versions. Their
// version implementations are defined in their respective packages.
// This package is needed to prevent import cycles.
type Obtain interface {
	Local() (string, error)
	Remote(host string) (string, error)
	Description() string
}
