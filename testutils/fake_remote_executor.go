package testutils

type StubRemoteExecutor struct {
	VerifySoftwareHosts chan []string
	StartHosts          chan []string
}

func NewStubRemoteExecutor() *StubRemoteExecutor {
	return &StubRemoteExecutor{
		VerifySoftwareHosts: make(chan []string, 1),
		StartHosts:          make(chan []string, 1),
	}
}

func (s *StubRemoteExecutor) VerifySoftware(hosts []string) {
	s.VerifySoftwareHosts <- hosts
}

func (s *StubRemoteExecutor) Start(hosts []string) {
	s.StartHosts <- hosts
}
