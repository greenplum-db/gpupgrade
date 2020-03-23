package state

import (
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

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

const ConfigFileName = "config.json"

// TODO: make private to package
func GetConfigFilepath(stateDir string) string {
	return filepath.Join(stateDir, ConfigFileName)
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
