package commanders

import (
	"context"
	"fmt"
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
		var msg *idl.ExecuteMessage
		msg, err = stream.Recv()
		if err != nil {
			break
		}

		switch x := msg.Contents.(type) {
		case *idl.ExecuteMessage_Chunk:
			if x.Chunk.Type == idl.Chunk_STDOUT {
				os.Stdout.Write(x.Chunk.Buffer)
			} else if x.Chunk.Type == idl.Chunk_STDERR {
				os.Stderr.Write(x.Chunk.Buffer)
			}

		case *idl.ExecuteMessage_Status:
			fmt.Printf("Step: %s, Status: %s\n", x.Status.Step, x.Status.Status)

		default:
			panic(fmt.Sprintf("Unknown message type for Execute: %T", x))
		}
	}

	if err != io.EOF {
		return err
	}

	return nil
}
