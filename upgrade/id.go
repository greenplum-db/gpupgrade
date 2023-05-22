// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// NewID creates a new unique upgrade ID that is reasonably unique across executions of the process.
func NewID() string {
	var bytes [8]byte // 64 bits

	for {
		// Use crypto/rand for this to avoid chicken-and-egg (i.e. what should we
		// seed math/rand with?). This is more expensive, but we expect this to be
		// called only once per upgrade anyway.
		_, err := rand.Read(bytes[:])
		if err != nil {
			// TODO: should we fall back in this case? It will be system-dependent.
			panic(fmt.Sprintf("unable to get random data: %+v", err))
		}

		// RawURLEncoding omits padding (which we don't need) and uses a
		// filesystem-safe character set.
		id := base64.RawURLEncoding.EncodeToString(bytes[:])

		// gpstart has a bug that doesn't handle "--" in directory names.
		// Until that is resolved, we need to generate IDs without "--".
		if strings.Contains(id, "--") {
			continue
		}

		return id
	}
}
