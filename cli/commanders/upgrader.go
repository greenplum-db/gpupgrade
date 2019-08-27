package commanders

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type Upgrader struct {
	client idl.CliToHubClient
}

func NewUpgrader(client idl.CliToHubClient) *Upgrader {
	return &Upgrader{client: client}
}

func (u *Upgrader) ConvertMaster() error {
	_, err := u.client.UpgradeConvertMaster(context.Background(), &idl.UpgradeConvertMasterRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("ERROR - Unable to connect to hub")
		return err
	}

	gplog.Info("Kicked off pg_upgrade request.")
	return nil
}

func (u *Upgrader) ConvertPrimaries() error {
	_, err := u.client.UpgradeConvertPrimaries(context.Background(), &idl.UpgradeConvertPrimariesRequest{})
	if err != nil {
		// TODO: Change the logging message?
		gplog.Error("Error when calling hub upgrade convert primaries: %v", err.Error())
		return err
	}

	gplog.Info("Kicked off pg_upgrade request for primaries")
	return nil
}

func (u *Upgrader) CopyMasterDataDir() error {
	_, err := u.client.UpgradeCopyMasterDataDir(context.Background(), &idl.UpgradeCopyMasterDataDirRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Kicked off request to copy master data directory ")
	return nil
}

func (u *Upgrader) ValidateStartCluster() error {
	_, err := u.client.UpgradeValidateStartCluster(context.Background(), &idl.UpgradeValidateStartClusterRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Kicked off request for validation of cluster startup")
	return nil
}

func (u *Upgrader) ReconfigurePorts() error {
	_, err := u.client.UpgradeReconfigurePorts(context.Background(), &idl.UpgradeReconfigurePortsRequest{})
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	gplog.Info("Request to reconfigure master port on upgraded cluster complete")
	return nil
}
