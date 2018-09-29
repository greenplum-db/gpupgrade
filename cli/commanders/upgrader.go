package commanders

import (
	"context"
	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type Upgrader struct {
	client pb.CliToHubClient
}

func NewUpgrader(client pb.CliToHubClient) *Upgrader {
	return &Upgrader{client: client}
}

func ConvertMasterCmd() error {
	client := connectToHub()
	return NewUpgrader(client).ConvertMaster()
}
func ConvertPrimariesCmd() error {
	client := connectToHub()
	return NewUpgrader(client).ConvertPrimaries()
}
func ReconfigurePortsCmd() error {
	client := connectToHub()
	return NewUpgrader(client).ReconfigurePorts()
}
func ShareOidsCmd() error {
	client := connectToHub()
	return NewUpgrader(client).ShareOids()
}
func ValidateStartClusterCmd() error {
	client := connectToHub()
	return NewUpgrader(client).ValidateStartCluster()
}
func (u *Upgrader) ConvertMaster() error {
	_, err := u.client.UpgradeConvertMaster(context.Background(), &pb.UpgradeConvertMasterRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	gplog.Info("Kicked off pg_upgrade request.")
	return nil
}

func (u *Upgrader) ConvertPrimaries() error {
	_, err := u.client.UpgradeConvertPrimaries(context.Background(), &pb.UpgradeConvertPrimariesRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("Error when calling hub upgrade convert primaries: %v", err.Error())
		return err
	}

	gplog.Info("Kicked off pg_upgrade request for primaries")
	return nil
}

func (u *Upgrader) ShareOids() error {
	_, err := u.client.UpgradeShareOids(context.Background(), &pb.UpgradeShareOidsRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Kicked off request to share oids")
	return nil
}

func (u *Upgrader) ValidateStartCluster() error {
	_, err := u.client.UpgradeValidateStartCluster(context.Background(), &pb.UpgradeValidateStartClusterRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Kicked off request for validation of cluster startup")
	return nil
}

func (u *Upgrader) ReconfigurePorts() error {
	_, err := u.client.UpgradeReconfigurePorts(context.Background(), &pb.UpgradeReconfigurePortsRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Request to reconfigure master port on upgraded cluster complete")
	return nil
}
