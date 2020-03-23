package state

import (
	"os"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
)

type State struct {
	StateDir string
	*Config
}

func (s *State) Save() (err error) {
	// TODO: Switch to an atomic implementation like renameio. Consider what
	// happens if config.Save() panics: we'll have truncated the file
	// on disk and the hub will be unable to recover. For now, since we normally
	// only save the configuration during initialize and any configuration
	// errors could be fixed by reinitializing, the risk seems small.
	file, err := utils.System.Create(GetConfigFilepath(s.StateDir))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			cerr = xerrors.Errorf("closing hub configuration: %w", cerr)
			err = multierror.Append(err, cerr).ErrorOrNil()
		}
	}()

	err = s.Config.Save(file)
	if err != nil {
		return xerrors.Errorf("saving hub configuration: %w", err)
	}

	return nil
}

func (s *State) Load() error {
	file, err := os.Open(GetConfigFilepath(s.StateDir))

	if err != nil {
		return xerrors.Errorf("opening configuration file: %w", err)
	}

	defer file.Close()

	err = s.Config.Load(file)

	if err != nil {
		return xerrors.Errorf("reading configuration file: %w", err)
	}

	return nil
}
