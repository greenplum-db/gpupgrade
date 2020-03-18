package hub

import (
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/context"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
)

func DeleteMirrorAndStandbyDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		mirrorsIncludingStandby := func(seg *greenplum.SegConfig) bool {
			return seg.Hostname == conn.Hostname &&
				(seg.Role == greenplum.MirrorRole)
		}

		segments := cluster.SelectSegments(mirrorsIncludingStandby)
		if len(segments) == 0 {
			// This can happen if there are no mirrors or standby on a host
			continue
		}

		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			req := new(idl.DeleteDirectoriesRequest)
			for _, seg := range segments {
				datadir := seg.DataDir
				req.Datadirs = append(req.Datadirs, datadir)
			}

			_, err := c.AgentClient.DeleteDirectories(context.Background(), req)
			if err != nil {
				gplog.Error("Error deleting segment data directories on host %s: %s",
					c.Hostname, err.Error())
				errChan <- err
			}
		}(conn)
	}

	wg.Wait()
	close(errChan)

	var mErr *multierror.Error
	for err := range errChan {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
