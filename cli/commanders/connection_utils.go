package commanders

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"google.golang.org/grpc"
)

// connTimeout retrieves the GPUPGRADE_CONNECTION_TIMEOUT environment variable,
// interprets it as a (possibly fractional) number of seconds, and converts it
// into a Duration. The default is one second if the envvar is unset or
// unreadable.
//
// TODO: should we make this a global --option instead?
func connTimeout() time.Duration {
	const defaultDuration = time.Second

	seconds, ok := os.LookupEnv("GPUPGRADE_CONNECTION_TIMEOUT")
	if !ok {
		return defaultDuration
	}

	duration, err := strconv.ParseFloat(seconds, 64)
	if err != nil {
		gplog.Warn(`GPUPGRADE_CONNECTION_TIMEOUT of "%s" is invalid (%s); using default of one second`,
			seconds, err)
		return defaultDuration
	}

	return time.Duration(duration * float64(time.Second))
}

// connectToHub() performs a blocking connection to the hub, and returns a
// CliToHubClient which wraps the resulting gRPC channel. Any errors result in
// an os.Exit(1).
func connectToHub() pb.CliToHubClient {
	upgradePort := os.Getenv("GPUPGRADE_HUB_PORT")
	if upgradePort == "" {
		upgradePort = "7527"
	}

	hubAddr := "localhost:" + upgradePort

	// Set up our timeout.
	ctx, cancel := context.WithTimeout(context.Background(), connTimeout())
	defer cancel()

	// Attempt a connection.
	conn, err := grpc.DialContext(ctx, hubAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		// Print a nicer error message if we can't connect to the hub.
		if ctx.Err() == context.DeadlineExceeded {
			gplog.Error("couldn't connect to the upgrade hub (did you run 'gpupgrade prepare start-hub'?)")
		} else {
			gplog.Error(err.Error())
		}
		os.Exit(1)
	}

	return pb.NewCliToHubClient(conn)
}
