package services

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	"google.golang.org/grpc"

	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

const (
	// todo generalize to any host
	port = "6416"
)

type ClientAndHostname struct {
	Client   pb.AgentClient
	Hostname string
}

type PingerManager struct {
	RPCClients       []ClientAndHostname
	NumRetries       int
	PauseBeforeRetry time.Duration
}

// nolint: unparam
// We leave stateDir as an argument for now because it doesn't function correctly at the moment,
// and we're going to refactor it later and we don't want to change all the calls now.
func NewPingerManager(stateDir string, t time.Duration) *PingerManager {
	// TODO: Do this *after* the hub exists
	//rpcClients := GetClients(pair.GetHostnames())
	//return &PingerManager{rpcClients, 10, t}
	return &PingerManager{[]ClientAndHostname{}, 10, t}
}

func GetClients(hostnames []string) []ClientAndHostname {
	var clients []ClientAndHostname
	for i := 0; i < len(hostnames); i++ {
		conn, err := grpc.Dial(hostnames[i]+":"+port, grpc.WithInsecure())
		if err != nil {
			gplog.Error(err.Error())
		}
		clientAndHost := ClientAndHostname{
			Client:   pb.NewAgentClient(conn),
			Hostname: hostnames[i],
		}
		clients = append(clients, clientAndHost)
	}
	return clients
}

func (agent *PingerManager) PingPollAgents() error {
	var err error
	done := false
	for i := 0; i < 10; i++ {
		gplog.Info("Pinging agents...")
		err = agent.PingAllAgents()
		if err == nil {
			done = true
			break
		}
		time.Sleep(agent.PauseBeforeRetry)
	}
	if !done {
		gplog.Info("Reached ping timeout")
	}
	return err
}

func (agent *PingerManager) PingAllAgents() error {
	//TODO: ping all the agents in parallel?
	for i := 0; i < len(agent.RPCClients); i++ {
		_, err := agent.RPCClients[i].Client.PingAgents(context.Background(), &pb.PingAgentsRequest{})
		if err != nil {
			gplog.Error("Not all agents on the segment hosts are running.")
			return err
		}
	}

	return nil
}
