package hub

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
)

type AgentBroker interface {
	ReconfigureDataDirectories(hostname string, renamePairs []*idl.RenamePair) error
}

type AgentBrokerGRPC struct {
	agentConnections map[string]*Connection
	context          context.Context
}

func NewAgentBroker(ctx context.Context, agentConnections []*Connection) *AgentBrokerGRPC {
	connectionsGroupedByHost := make(map[string]*Connection)

	for _, connection := range agentConnections {
		connectionsGroupedByHost[connection.Hostname] = connection
	}

	return &AgentBrokerGRPC{
		context:          ctx,
		agentConnections: connectionsGroupedByHost,
	}
}

//
//
// ensure that this function remains goroutine safe
//
func (broker *AgentBrokerGRPC) ReconfigureDataDirectories(hostname string, renamePairs []*idl.RenamePair) error {
	var connection *Connection

	if connection = broker.agentConnections[hostname]; connection == nil {
		return errors.New(fmt.Sprintf("No agent connections for hostname=%v", hostname))
	}

	if len(renamePairs) == 0 {
		return nil
	}

	_, err := connection.AgentClient.ReconfigureDataDirectories(
		broker.context,
		&idl.ReconfigureDataDirRequest{
			Pairs: renamePairs,
		},
	)

	return err
}
