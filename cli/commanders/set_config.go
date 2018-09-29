package commanders

import (
	"context"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

type ConfigSetter struct {
	client pb.CliToHubClient
}

/* separate out client here for Mocking...*/
/* NOTE: since we are passed in a map, the order of setting is random */
func NewConfigSetterCmd(flagMap map[string]string) error {
	client := connectToHub()
	return NewConfigSetter(client).Execute(flagMap)
}

func NewConfigSetter(client pb.CliToHubClient) ConfigSetter {
	return ConfigSetter{
		client: client,
	}
}

func (req ConfigSetter) Execute(flagMap map[string]string) error {

	var requests []*pb.SetConfigRequest
	for name, value := range flagMap {
		requests = append(requests, &pb.SetConfigRequest{
			Name:  name,
			Value: value,
		})
	}

	for _, request := range requests {
		_, err := req.client.SetConfig(context.Background(), request)
		if err != nil {
			return err
		}
		gplog.Info("Successfully set %s to %s", request.Name, request.Value)
	}

	return nil
}
