package agent

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

var DialTimeout = 3 * time.Second

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)
