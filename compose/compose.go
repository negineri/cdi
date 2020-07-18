package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

type (
	services struct {
		Image       string   `json:"image"`
		Restart     string   `json:"restart"`
		Environment []string `json:"environment"`
		Volumes     []string `json:"volumes"`
		ExposePort  string   `json:"expose_port"`
	}
	volumes struct {
	}
	compose struct {
		Version  string              `json:"version"`
		Services map[string]services `json:"services"`
		Volumes  map[string]volumes  `json:"volumes"`
	}
	userConfig struct {
		stackName string
		route     string
		userID    string
	}
)

func NewStack(ctx context.Context, cli *client.Client, userConfig userConfig) error {
	bytes, err := ioutil.ReadFile("stacks/" + userConfig.stackName + ".json")
	if err != nil {
		return err
	}
	config := compose{}
	if err := json.Unmarshal(bytes, &config); err != nil {
		return err
	}
	for sName, service := range config.Services {
		fmt.Printf("%s\n", sName)
		cConfig := container.Config{AttachStdout: false, AttachStderr: false}
		hConfig := container.HostConfig{
			LogConfig: container.LogConfig{Type: "json-file"}}
		cConfig.Image = service.Image
		cConfig.Env = append(cConfig.Env, service.Environment...)
		for _, cVolume := range service.Volumes {
			ss := strings.Split(cVolume, ":")
			switch len(ss) {
			case 2:
				cConfig.Volumes[ss[1]] = volumes{}
				hConfig.Binds = append(hConfig.Binds, cVolume)
				if strings.HasPrefix(ss[0], ".") {
					hConfig.Binds = append(hConfig.Binds)
				} else {

					cli.VolumeCreate(ctx, volume.VolumeCreateBody{
						Name: userConfig.userID + "_" + userConfig.stackName + "_" + ss[0]})
					hConfig.Mounts = append(hConfig.Mounts,
						mount.Mount{Target: ss[1],
							Source:   ss[0],
							Type:     "volume",
							ReadOnly: false,
						})
				}
			default:
				return errors.New("TO DO")
			}
		}
	}
}
