package hub

import (
	"github.com/greenplum-db/gpupgrade/hub/finalize"

	"github.com/greenplum-db/gpupgrade/idl"
)

func (s *Server) Finalize(_ *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	return finalize.Finalize(stream, s.StateDir, s.Source, s.Target)
}
