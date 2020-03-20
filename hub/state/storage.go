package state

import (
	"encoding/json"
	"io"
	"os"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"

type InitializeConfig struct {
	Standby   greenplum.SegConfig
	Master    greenplum.SegConfig
	Primaries []greenplum.SegConfig
	Mirrors   []greenplum.SegConfig
}

type Config struct {
	Source *greenplum.Cluster
	Target *greenplum.Cluster

	// TargetInitializeConfig contains all the info needed to initialize the
	// target cluster's master, standby, primaries and mirrors.
	TargetInitializeConfig InitializeConfig

	Port        int
	AgentPort   int
	UseLinkMode bool
}

// Save persists the hub's configuration to disk.
func Save(stateDir string, config *Config) (err error) {
	// TODO: Switch to an atomic implementation like renameio. Consider what
	// happens if Config.Save() panics: we'll have truncated the file
	// on disk and the hub will be unable to recover. For now, since we normally
	// only save the configuration during initialize and any configuration
	// errors could be fixed by reinitializing, the risk seems small.
	file, err := utils.System.Create(GetConfigFilepath(stateDir))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			cerr = xerrors.Errorf("closing hub configuration: %w", cerr)
			err = multierror.Append(err, cerr).ErrorOrNil()
		}
	}()

	err = config.Save(file)
	if err != nil {
		return xerrors.Errorf("saving hub configuration: %w", err)
	}

	return nil
}

// Config contains all the information that will be persisted to/loaded from
// from disk during calls to Save() and Load().
func (c *Config) Load(r io.Reader) error {
	dec := json.NewDecoder(r)
	return dec.Decode(c)
}

func (c *Config) Save(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

func LoadConfig(conf *Config, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("opening configuration file: %w", err)
	}
	defer file.Close()

	err = conf.Load(file)
	if err != nil {
		return xerrors.Errorf("reading configuration file: %w", err)
	}

	return nil
}
