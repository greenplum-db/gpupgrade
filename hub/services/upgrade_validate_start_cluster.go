package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/pkg/errors"
)

func (h *Hub) UpgradeValidateStartCluster(ctx context.Context, in *idl.UpgradeValidateStartClusterRequest) (*idl.UpgradeValidateStartClusterReply, error) {
	gplog.Info("starting %s", upgradestatus.VALIDATE_START_CLUSTER)
	defer log.WritePanics()

	stepWriter, err := h.WriteStep(upgradestatus.VALIDATE_START_CLUSTER)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.UpgradeValidateStartClusterReply{}, err
	}

	err = h.startNewCluster()
	if err != nil {
		gplog.Error(err.Error())
		stepWriter.MarkFailed()
		return &idl.UpgradeValidateStartClusterReply{}, err
	}

	stepWriter.MarkComplete()
	return &idl.UpgradeValidateStartClusterReply{}, nil
}

func (h *Hub) startNewCluster() error {
	startCmd := fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstart -a -d %s", h.target.BinDir, h.target.BinDir, h.target.MasterDataDir())
	_, err := h.target.ExecuteLocalCommand(startCmd)
	if err != nil {
		return errors.Wrap(err, "failed to start new cluster")
	}

	return nil
}
