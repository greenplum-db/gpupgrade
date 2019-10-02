package commanders

import (
	"context"
	"io"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
)

func Execute(client idl.CliToHubClient) error {
	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	for {
		var streamType *idl.UpgradeStream
		streamType, err = stream.Recv()
		if err == nil {
			if streamType.GetType() == idl.UpgradeStream_CHUNK {
				chunk := streamType.GetChunk()
				if chunk.Type == idl.Chunk_STDOUT {
					os.Stdout.Write(chunk.Buffer)
				} else if chunk.Type == idl.Chunk_STDERR {
					os.Stderr.Write(chunk.Buffer)
				}
			}
		} else {
			break
		}
	}

	if err != io.EOF {
		return err
	}

	return nil
}
