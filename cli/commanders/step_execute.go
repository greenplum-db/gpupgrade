package commanders

import (
	"context"
	"io"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
)

func Execute(client idl.CliToHubClient, verbose bool) error {
	stream, err := client.Execute(context.Background(), &idl.ExecuteRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	os.Stdout.WriteString("Execute in progress")

	for {
		var chunk *idl.Chunk
		chunk, err = stream.Recv()
		if err != nil {
			break
		}

		if verbose {
			if chunk.Type == idl.Chunk_STDOUT {
				os.Stdout.Write(chunk.Buffer)
			} else if chunk.Type == idl.Chunk_STDERR {
				os.Stderr.Write(chunk.Buffer)
			}
		}
	}

	if err != io.EOF {
		return err
	}

	return nil
}
