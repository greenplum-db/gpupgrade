package commanders

import (
	"context"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

type ConfigShower struct {
	client pb.CliToHubClient
}

/* separate out client here for Mocking...*/
func NewConfigShowerCmd(flagList []string) (map[string]string, error) {
	client := connectToHub()
	return NewConfigShower(client).Execute(flagList)
}

func NewConfigShower(client pb.CliToHubClient) ConfigShower {
	return ConfigShower{
		client: client,
	}
}

func (req ConfigShower) Execute(flagList []string) (map[string]string, error) {

	configMap := make(map[string]string)

	// Build a list of GetConfigRequests, one for each flag. If no flags
	// are passed, assume we want to retrieve all of them.
	var requests []*pb.GetConfigRequest
	for _, name := range flagList {
		requests = append(requests, &pb.GetConfigRequest{
			Name: name,
		})
	}

	// Make the requests and print every response.
	for _, request := range requests {
		resp, err := req.client.GetConfig(context.Background(), request)
		if err != nil {
			return configMap, err
		}
		configMap[request.Name] = resp.Value
	}

	return configMap, nil
}
