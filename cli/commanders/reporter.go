package commanders

import (
	"context"
	"fmt"
	"sort"

	"github.com/greenplum-db/gpupgrade/idl"

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
	idl.UpgradeSteps_SEGINSTALL:             "- Install binaries on segments",
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

type PrimaryStatuses []*idl.PrimaryStatus

func (s PrimaryStatuses) Len() int {
	return len(s)
}

func (s PrimaryStatuses) Less(i, j int) bool {
	return s[i].Dbid < s[j].Dbid
}

func (s PrimaryStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

/*
 * This map, and the associated UpgradeStepStatus sorting functions below,
 * enable sorting gpupgrade status at the CLI so that the hub and agents do not
 * need to be recompiled and restarted to change the display order.
 */
var UpgradeStepsOrder = map[idl.UpgradeSteps]int{
	idl.UpgradeSteps_UNKNOWN_STEP:           0,
	idl.UpgradeSteps_CONFIG:                 1,
	idl.UpgradeSteps_SEGINSTALL:             2,
	idl.UpgradeSteps_START_AGENTS:           3,
	idl.UpgradeSteps_INIT_CLUSTER:           4,
	idl.UpgradeSteps_SHUTDOWN_CLUSTERS:      5,
	idl.UpgradeSteps_CONVERT_MASTER:         6,
	idl.UpgradeSteps_COPY_MASTER:            7,
	idl.UpgradeSteps_CONVERT_PRIMARIES:      8,
	idl.UpgradeSteps_VALIDATE_START_CLUSTER: 9,
	idl.UpgradeSteps_RECONFIGURE_PORTS:      10,
}

type StepStatuses []*idl.UpgradeStepStatus

func (s StepStatuses) Len() int {
	return len(s)
}

func (s StepStatuses) Less(i, j int) bool {
	iStep := s[i].GetStep()
	jStep := s[j].GetStep()
	return UpgradeStepsOrder[iStep] < UpgradeStepsOrder[jStep]
}

func (s StepStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
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
	sort.Sort(StepStatuses(statuses))
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
	sort.Sort(PrimaryStatuses(statuses))
	formatStr := "%s - DBID %d - CONTENT ID %d - PRIMARY - %s"

	for _, status := range statuses {
		reportString := fmt.Sprintf(formatStr, status.Status, status.Dbid, status.Content, status.Hostname)
		fmt.Println(reportString)
	}

	return nil
}
