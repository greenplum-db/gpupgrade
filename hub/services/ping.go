package services

import (
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) Ping(ctx context.Context, in *idl.PingRequest) (*idl.PingReply, error) {
	gplog.Info("starting Ping")
	return &idl.PingReply{}, nil
}
