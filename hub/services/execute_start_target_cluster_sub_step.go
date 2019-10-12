package services

import (
	"fmt"
	"github.com/greenplum-db/gpupgrade/idl"
	"io"
)

func (h *Hub) StartTargetCluster(stream messageSender, log io.Writer) error {
	cmd := execCommand("bash", "-c",
		fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/gpstart -a -d %[2]s",
			h.target.BinDir,
			h.target.MasterDataDir(),
		))

	mux := newMultiplexedStream(stream, log)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	return cmd.Run()
}
