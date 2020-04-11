package hub

import (
	"context"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/hashicorp/go-multierror"
)

//func (s *Server) DeleteDirectories(ctx context.Context, in *idl.DeleteDirectoriesRequest) (*idl.DeleteDirectoriesReply, error) {
//func (s *Server) DeleteDirectory(ctx context.Context, in *idl.DeleteDirectoryRequest) (*idl.DeleteDirectoryReply, error) {
func DeleteStateDirectories(agentConns []*Connection, masterHostName string) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))
	stateDir := utils.GetStateDir()

	for _, conn := range agentConns {
		conn := conn


		if conn.Hostname == masterHostName {
			continue
		}

		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			request := &idl.DeleteStateDirectoryRequest{Directory: stateDir}
			_, err := c.AgentClient.DeleteStateDirectory(context.Background(), request)
			if err != nil {
				gplog.Error("Error deleting state directory on host %s: %s",
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