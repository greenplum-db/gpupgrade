package services

import (
	"io"
	"sync"

	"github.com/hashicorp/go-multierror"
)

func (h *Hub) CheckUpgrade(stream messageSender, log io.Writer) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := h.UpgradeMaster(stream, log, true)
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
