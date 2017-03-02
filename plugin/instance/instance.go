package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	log "github.com/Sirupsen/logrus"
	apitypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/infrakit/pkg/spi"
	"github.com/docker/infrakit/pkg/spi/instance"
	"github.com/docker/infrakit/pkg/types"
	"golang.org/x/net/context"
)

type dockerInstancePlugin struct {
	client        client.Client
	ctx           context.Context
	namespaceTags map[string]string
}

type properties struct {
	Host     string
	Retries  int
	Instance *types.Any
}

// NewInstancePlugin creates a new plugin that creates instances on the Docker host
func NewInstancePlugin(client *client.Client, namespaceTags map[string]string) instance.Plugin {
	return &dockerInstancePlugin{client: *client, ctx: context.Background(), namespaceTags: namespaceTags}
}

func (p dockerInstancePlugin) tagInstance(
	id *(instance.ID),
	systemTags map[string]string,
	userTags map[string]string) error {
	// todo
	return nil
}

// CreateInstanceRequest is the concrete provision request type.
type CreateInstanceRequest struct {
	Tags             map[string]string
	Config           *container.Config
	HostConfig       *container.HostConfig
	NetworkingConfig *network.NetworkingConfig
	NetworkName      string
}

// VendorInfo returns a vendor specific name and version
func (p dockerInstancePlugin) VendorInfo() *spi.VendorInfo {
	return &spi.VendorInfo{
		InterfaceSpec: spi.InterfaceSpec{
			Name:    "infrakit-instance-docker",
			Version: "0.3.0",
		},
		URL: "https://github.com/docker/infrakit.docker",
	}
}

// ExampleProperties returns the properties / config of this plugin
func (p dockerInstancePlugin) ExampleProperties() *types.Any {
	config := container.Config{
		Image: "docker/dind",
		Env: []string{
			"var1=value1",
			"var2=value2",
		},
	}
	hostConfig := container.HostConfig{}
	networkingConfig := network.NetworkingConfig{}
	example := CreateInstanceRequest{
		Tags: map[string]string{
			"tag1": "value1",
			"tag2": "value2",
		},
		Config:           &config,
		HostConfig:       &hostConfig,
		NetworkingConfig: &networkingConfig,
	}

	any, err := types.AnyValue(example)
	if err != nil {
		panic(err)
	}
	return any
}

// Validate performs local checks to determine if the request is valid
func (p dockerInstancePlugin) Validate(req *types.Any) error {
	return nil
}

// Label implements labeling the instances
func (p dockerInstancePlugin) Label(id instance.ID, labels map[string]string) error {
	return fmt.Errorf("Docker container label updates are not implemented yet")
}

// mergeTags merges multiple maps of tags, implementing 'last write wins' for colliding keys.
//
// Returns a sorted slice of all keys, and the map of merged tags.  Sorted keys are particularly useful to assist in
// preparing predictable output such as for tests.
func mergeTags(tagMaps ...map[string]string) ([]string, map[string]string) {

	keys := []string{}
	tags := map[string]string{}

	for _, tagMap := range tagMaps {
		for k, v := range tagMap {
			if _, exists := tags[k]; exists {
				log.Warnf("Ovewriting tag value for key %s", k)
			} else {
				keys = append(keys, k)
			}
			tags[k] = v
		}
	}

	sort.Strings(keys)

	return keys, tags
}

// Provision creates a new instance
func (p dockerInstancePlugin) Provision(spec instance.Spec) (*instance.ID, error) {

	if spec.Properties == nil {
		return nil, errors.New("Properties must be set")
	}

	request := CreateInstanceRequest{}
	err := json.Unmarshal(*spec.Properties, &request)
	if err != nil {
		return nil, fmt.Errorf("invalid input formatting: %s", err)
	}
	if request.Config == nil {
		return nil, errors.New("Config should be set")
	}
	// merge tags
	_, allTags := mergeTags(spec.Tags, request.Tags)
	request.Config.Labels = allTags

	if spec.LogicalID != nil {
		if request.NetworkName == "" {
			request.NetworkName = "bridge"
		}
		if request.NetworkingConfig == nil {
			request.NetworkingConfig = &network.NetworkingConfig{}
		}
		if request.NetworkingConfig.EndpointsConfig == nil {
			request.NetworkingConfig.EndpointsConfig = map[string]*(network.EndpointSettings){}
		}
		if request.NetworkingConfig.EndpointsConfig[request.NetworkName] == nil {
			request.NetworkingConfig.EndpointsConfig[request.NetworkName] = &network.EndpointSettings{}
		}
		request.NetworkingConfig.EndpointsConfig[request.NetworkName].IPAddress = (string)(*spec.LogicalID)
	}

	cli := p.client
	ctx := context.Background()
	image := request.Config.Image
	if image == "" {
		return nil, errors.New("A Docker image should be specified")
	}
	reader, err := cli.ImagePull(ctx, image, apitypes.ImagePullOptions{})
	if err != nil {
		return nil, err
	}
	data := make([]byte, 1000, 1000)
	for {
		_, err := reader.Read(data)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	containerName := ""
	r, err := cli.ContainerCreate(ctx, request.Config, request.HostConfig, request.NetworkingConfig, containerName)
	if err != nil {
		return nil, err
	}

	if r.ID == "" {
		return nil, errors.New("Unexpected Docker API response")
	}
	id := (instance.ID)(r.ID)

	return &id, nil
}

// Destroy terminates an existing instance
func (p dockerInstancePlugin) Destroy(id instance.ID) error {
	options := apitypes.ContainerRemoveOptions{Force: true, RemoveVolumes: true, RemoveLinks: false}
	cli := p.client
	ctx := context.Background()
	if err := cli.ContainerRemove(ctx, string(id), options); err != nil {
		return err
	}
	return nil
}

func describeGroupRequest(namespaceTags, tags map[string]string) *apitypes.ContainerListOptions {

	filter := filters.NewArgs()
	filter.Add("status", "created")
	filter.Add("status", "running")

	keys, allTags := mergeTags(tags, namespaceTags)

	for _, key := range keys {
		filter.Add("label", fmt.Sprintf("%s=%s", key, allTags[key]))
	}
	options := apitypes.ContainerListOptions{
		Filters: filter,
	}
	return &options
}

func (p dockerInstancePlugin) describeInstances(tags map[string]string) ([]instance.Description, error) {

	options := describeGroupRequest(p.namespaceTags, tags)
	ctx := context.Background()
	containers, err := p.client.ContainerList(ctx, *options)
	if err != nil {
		return nil, err
	}

	descriptions := []instance.Description{}
	var ns map[string]*network.EndpointSettings
	for _, container := range containers {
		tags := map[string]string{}
		if container.Labels != nil {
			for key, value := range container.Labels {
				tags[key] = value
			}
		}
		ns = container.NetworkSettings.Networks
		if ns != nil {
			for _, eps := range ns {
				lid := (instance.LogicalID)(eps.IPAddress)
				descriptions = append(descriptions, instance.Description{
					ID:        instance.ID(container.ID),
					LogicalID: &lid,
					Tags:      tags,
				})
				break
			}
		} else {
			descriptions = append(descriptions, instance.Description{
				ID:   instance.ID(container.ID),
				Tags: tags,
			})
		}
	}

	return descriptions, nil
}

// DescribeInstances implements instance.Provisioner.DescribeInstances.
func (p dockerInstancePlugin) DescribeInstances(tags map[string]string) ([]instance.Description, error) {
	return p.describeInstances(tags)
}
