package commanders

import (
	"context"
	"fmt"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
)

type Reporter struct {
	client pb.CliToHubClient
}

// UpgradeStepsMessage encode the proper checklist item string to go with a step
//
// Future steps include:
//logger.Info("PENDING - Validate compatible versions for upgrade")
//logger.Info("PENDING - Master server upgrade")
//logger.Info("PENDING - Primary segment upgrade")
//logger.Info("PENDING - Validate cluster start")
//logger.Info("PENDING - Adjust upgrade cluster ports")
var UpgradeStepsMessage = map[pb.UpgradeSteps]string{
	pb.UpgradeSteps_UNKNOWN_STEP:           "- Unknown step",
	pb.UpgradeSteps_CONFIG:                 "- Configuration Check",
	pb.UpgradeSteps_SEGINSTALL:             "- Install binaries on segments",
	pb.UpgradeSteps_INIT_CLUSTER:           "- Initialize upgrade target cluster",
	pb.UpgradeSteps_START_AGENTS:           "- Agents Started on Cluster",
	pb.UpgradeSteps_CONVERT_MASTER:         "- Run pg_upgrade on master",
	pb.UpgradeSteps_SHUTDOWN_CLUSTERS:      "- Shutdown clusters",
	pb.UpgradeSteps_SHARE_OIDS:             "- Copy OID files from master to segments",
	pb.UpgradeSteps_CONVERT_PRIMARIES:      "- Run pg_upgrade on primaries",
	pb.UpgradeSteps_VALIDATE_START_CLUSTER: "- Validate the upgraded cluster can start up",
	pb.UpgradeSteps_RECONFIGURE_PORTS:      "- Adjust upgraded cluster ports",
}

func NewReporter(client pb.CliToHubClient) *Reporter {
	return &Reporter{
		client: client,
	}
}

func (r *Reporter) OverallUpgradeStatus() error {
	status, err := r.client.StatusUpgrade(context.Background(), &pb.StatusUpgradeRequest{})
	if err != nil {
		// find some way to expound on the error message? Integration test failing because we no longer log here
		return errors.New("Failed to retrieve status from hub: " + err.Error())
	}

	if len(status.GetListOfUpgradeStepStatuses()) == 0 {
		return errors.New("Received no list of upgrade statuses from hub")
	}

	for _, step := range status.GetListOfUpgradeStepStatuses() {
		reportString := fmt.Sprintf("%v %s", step.GetStatus(),
			UpgradeStepsMessage[step.GetStep()])
		gplog.Info(reportString)
	}

	return nil
}

func (r *Reporter) OverallConversionStatus() error {
	conversionStatus, err := r.client.StatusConversion(context.Background(), &pb.StatusConversionRequest{})
	if err != nil {
		return errors.New("hub returned an error when checking overall conversion status: " + err.Error())
	}

	if len(conversionStatus.GetConversionStatuses()) == 0 {
		return errors.New("Received no list of conversion statuses from hub")
	}

	for _, status := range conversionStatus.GetConversionStatuses() {
		gplog.Info(status)
	}

	return nil
}
