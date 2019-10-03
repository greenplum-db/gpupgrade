package services

import (
	"github.com/greenplum-db/gpupgrade/idl"
)

func (h *Hub) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) error {
	err := h.ExecuteInitTargetClusterSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteShutdownClustersSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteUpgradeMasterSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteCopyMasterSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteUpgradePrimariesSubStep(stream)
	if err != nil {
		return err
	}

	err = h.ExecuteStartTargetClusterSubStep(stream)
	return err
}
