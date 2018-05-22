package commanders

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type Preparer struct {
	client pb.CliToHubClient
}

func NewPreparer(client pb.CliToHubClient) Preparer {
	return Preparer{client: client}
}

var NumberOfConnectionAttempt = 100

func (p Preparer) ShutdownClusters(oldBinDir string, newBinDir string) error {
	_, err := p.client.PrepareShutdownClusters(context.Background(),
		&pb.PrepareShutdownClustersRequest{OldBinDir: oldBinDir, NewBinDir: newBinDir})
	if err != nil {
		gplog.Error(err.Error())
	}
	gplog.Info("request to shutdown clusters sent to hub")
	return nil
}

func (p Preparer) StartHub() error {
	countHubs, err := HowManyHubsRunning()
	if err != nil {
		gplog.Error("failed to determine if hub already running")
		return err
	}
	if countHubs >= 1 {
		gplog.Error("gpupgrade_hub process already running")
		return errors.New("gpupgrade_hub process already running")
	}

	//assume that gpupgrade_hub is on the PATH
	cmd := exec.Command("gpupgrade_hub")
	cmdErr := cmd.Start()
	if cmdErr != nil {
		gplog.Error("gpupgrade_hub kickoff failed")
		return cmdErr
	}
	gplog.Debug("gpupgrade_hub started")
	return nil
}

func (p Preparer) InitCluster(dbPort int, newBinDir string) error {
	_, err := p.client.PrepareInitCluster(context.Background(), &pb.PrepareInitClusterRequest{DbPort: int32(dbPort), NewBinDir: newBinDir})
	if err != nil {
		return err
	}

	gplog.Info("Gleaning the new cluster config")
	return nil
}

func (p Preparer) VerifyConnectivity(client pb.CliToHubClient) error {
	_, err := client.Ping(context.Background(), &pb.PingRequest{})
	for i := 0; i < NumberOfConnectionAttempt && err != nil; i++ {
		_, err = client.Ping(context.Background(), &pb.PingRequest{})
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func (p Preparer) StartAgents() error {
	_, err := p.client.PrepareStartAgents(context.Background(), &pb.PrepareStartAgentsRequest{})
	if err != nil {
		return err
	}

	gplog.Info("Started Agents in progress, check gpupgrade_agent logs for details")
	return nil
}

func HowManyHubsRunning() (int, error) {
	howToLookForHub := `ps -ef | grep -Gc "[g]pupgrade_hub$"` // use square brackets to avoid finding yourself in matches
	output, err := exec.Command("bash", "-c", howToLookForHub).Output()
	value, convErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if convErr != nil {
		if err != nil {
			return -1, err
		}
		return -1, convErr
	}

	// let value == 0 through before checking err, for when grep finds nothing and its error-code is 1
	if value >= 0 {
		return value, nil
	}

	// only needed if the command errors, but somehow put a parsable & negative value on stdout
	return -1, err
}
