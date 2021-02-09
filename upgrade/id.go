// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

// ID is a unique identifier for a cluster upgrade.
type ID uint64

// NewID creates a new unique ID. It should be reasonably unique across
// executions of the process.
func NewID() ID {
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

		num := binary.LittleEndian.Uint64(bytes[:])

		// gpstart has a bug that doesn't handle "--" in directory names.
		// Until that is resolved, we need to generate IDs without "--".
		if strings.Contains(ID(num).String(), "--") {
			continue
		}

		return ID(num)
	}
}

// String returns an unpadded, filesystem-safe base64 encoding of the
// identifier.
func (id ID) String() string {
	var bytes [8]byte // 64 bits
	binary.LittleEndian.PutUint64(bytes[:], uint64(id))

	// RawURLEncoding omits padding (which we don't need) and uses a
	// filesystem-safe character set.
	return base64.RawURLEncoding.EncodeToString(bytes[:])
}
