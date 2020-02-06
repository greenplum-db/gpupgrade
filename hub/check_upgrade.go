package hub

import (
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/step"
)

func (s *Server) CheckUpgrade(stream step.OutStreams) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()

		stateDir := s.StateDir
		err := UpgradeMaster(s.Source, s.Target, stateDir, stream, true, false)
		if err != nil {
			checkErrs <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := s.ConvertPrimaries(true)
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
