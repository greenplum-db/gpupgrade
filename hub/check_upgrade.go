package hub

import (
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/step"
)

func (h *Server) CheckUpgrade(stream step.OutStreams) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()

		stateDir := h.StateDir
		err := UpgradeMaster(h.Source, h.Target, stateDir, stream, true, false)
		if err != nil {
			checkErrs <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := h.ConvertPrimaries(true)
		if err != nil {
			checkErrs <- err
		}
	}()

	wg.Wait()
	close(checkErrs)

	var multiErr *multierror.Error
	for err := range checkErrs {
		multiErr = multierror.Append(multiErr, err)
	}

	return multiErr.ErrorOrNil()
}
