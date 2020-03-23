package state

type State struct {
	StateDir string
	*Config
}

func (s *State) Save() error {
	return save(s.StateDir, s.Config)
}

func (s *State) Load() error {
	return loadConfig(s.Config,
		GetConfigFilepath(s.StateDir))
}
