package agent

import (
	"context"

	"google.golang.org/grpc"
)

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)
