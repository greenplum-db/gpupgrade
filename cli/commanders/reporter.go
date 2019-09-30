package commanders

import (
	"context"
	"fmt"
	"sort"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"
)

type Reporter struct {
	client idl.CliToHubClient
}

// UpgradeStepsMessage encode the proper checklist item string to go with a step
//
// Future steps include:
//logger.Info("PENDING - Validate compatible versions for upgrade")
//logger.Info("PENDING - Master server upgrade")
//logger.Info("PENDING - Primary segment upgrade")
//logger.Info("PENDING - Validate cluster start")
//logger.Info("PENDING - Adjust upgrade cluster ports")
var UpgradeStepsMessage = map[idl.UpgradeSteps]string{
	idl.UpgradeSteps_UNKNOWN_STEP:           "- Unknown step",
	idl.UpgradeSteps_CONFIG:                 "- Configuration Check",
	idl.UpgradeSteps_START_AGENTS:           "- Agents Started on Cluster",
	idl.UpgradeSteps_INIT_CLUSTER:           "- Initialize new cluster",
	idl.UpgradeSteps_CONVERT_MASTER:         "- Run pg_upgrade on master",
	idl.UpgradeSteps_SHUTDOWN_CLUSTERS:      "- Shutdown clusters",
	idl.UpgradeSteps_COPY_MASTER:            "- Copy master data directory to segments",
	idl.UpgradeSteps_CONVERT_PRIMARIES:      "- Run pg_upgrade on primaries",
	idl.UpgradeSteps_VALIDATE_START_CLUSTER: "- Validate the upgraded cluster can start up",
	idl.UpgradeSteps_RECONFIGURE_PORTS:      "- Adjust upgraded cluster ports",
}

func NewReporter(client idl.CliToHubClient) *Reporter {
	return &Reporter{
		client: client,
	}
}

func (r *Reporter) OverallUpgradeStatus() error {
	status, err := r.client.StatusUpgrade(context.Background(), &idl.StatusUpgradeRequest{})
	if err != nil {
		// find some way to expound on the error message? Integration test failing because we no longer log here
		return errors.New("Failed to retrieve status from hub: " + err.Error())
	}

	if len(status.GetListOfUpgradeStepStatuses()) == 0 {
		return errors.New("Received no list of upgrade statuses from hub")
	}

	statuses := status.GetListOfUpgradeStepStatuses()
	sort.Sort(utils.StepStatuses(statuses))
	for _, step := range statuses {
		reportString := fmt.Sprintf("%v %s", step.GetStatus(),
			UpgradeStepsMessage[step.GetStep()])
		fmt.Println(reportString)
	}

	return nil
}

func (r *Reporter) OverallConversionStatus() error {
	conversionStatus, err := r.client.StatusConversion(context.Background(), &idl.StatusConversionRequest{})
	if err != nil {
		return errors.New("hub returned an error when checking overall conversion status: " + err.Error())
	}

	if len(conversionStatus.GetConversionStatuses()) == 0 {
		return errors.New("Received no list of conversion statuses from hub")
	}

	statuses := conversionStatus.GetConversionStatuses()
	sort.Sort(utils.PrimaryStatuses(statuses))
	formatStr := "%s - DBID %d - CONTENT ID %d - PRIMARY - %s"

	for _, status := range statuses {
		reportString := fmt.Sprintf(formatStr, status.Status, status.Dbid, status.Content, status.Hostname)
		fmt.Println(reportString)
	}

	return nil
}
