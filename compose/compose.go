package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/rs/xid"
)

type (
	services struct {
		Image       string   `json:"image"`
		Restart     string   `json:"restart"`
		Environment []string `json:"environment"`
		Volumes     []string `json:"volumes"`
		StandbyPort string   `json:"standby_port"`
		DependsOn   []string `json:"depends_on"`
		WorkingDir  string   `json:"working_dir"`
	}
	volumes struct {
		Driver     string            `json:"driver"`
		DriverOpts map[string]string `json:"driver_opts"`
	}
	chown struct {
	}
	compose struct {
		Version  string              `json:"version"`
		Services map[string]services `json:"services"`
		Volumes  map[string]volumes  `json:"volumes"`
		Chown    map[string]chown    `json:"chown"`
	}
	UserConfig struct {
		StackName string
		Route     string
		UserID    string
		UID       string
	}
)

func NewStack(ctx context.Context, cli *client.Client, userConfig UserConfig) error {
	jBytes, err := ioutil.ReadFile("stacks/" + userConfig.StackName + ".json")
	if err != nil {
		return err
	}
	jString := strings.Replace(string(jBytes), "${USER_ID}", userConfig.UserID, -1)
	jString = strings.Replace(jString, "${STACK_ROUTE}", userConfig.Route, -1)
	config := compose{}
	if err := json.Unmarshal([]byte(jString), &config); err != nil {
		return err
	}
	stackName := userConfig.UserID + "_" + userConfig.StackName

	if _, err := cli.NetworkCreate(ctx, stackName+"_default", types.NetworkCreate{}); err != nil {
		return err
	}
	cVolumes := make(map[string]string)
	for vName, vOpt := range config.Volumes {
		dName := stackName + "_" + vName
		vOption := volume.VolumeCreateBody{
			Name: dName, Driver: vOpt.Driver,
			DriverOpts: vOpt.DriverOpts}
		cli.VolumeCreate(ctx, vOption)
		cVolumes[vName] = dName
	}
	for sName, service := range config.Services {
		fmt.Printf("%s\n", sName)
		cName := stackName + "_" + sName
		cConfig := container.Config{AttachStdout: false,
			AttachStderr: false,
			Volumes:      make(map[string]struct{}),
			Labels:       make(map[string]string)}
		hConfig := container.HostConfig{
			LogConfig: container.LogConfig{Type: "json-file"}}
		nConfig := network.NetworkingConfig{EndpointsConfig: make(map[string]*network.EndpointSettings)}
		nConfig.EndpointsConfig[stackName+"_default"] = &network.EndpointSettings{Aliases: []string{sName}}
		cConfig.Image = service.Image
		cConfig.WorkingDir = service.WorkingDir
		if _, _, err := cli.ImageInspectWithRaw(ctx, cConfig.Image); err != nil {
			rc, err := cli.ImagePull(ctx, "docker.io/library/"+cConfig.Image, types.ImagePullOptions{})
			io.Copy(os.Stdout, rc)
			defer rc.Close()
			if err != nil {
				return nil
			}
		}
		cConfig.Env = append(cConfig.Env, service.Environment...)
		for _, cVolume := range service.Volumes {
			ss := strings.Split(cVolume, ":")
			switch len(ss) {
			case 2:
				cConfig.Volumes[ss[1]] = struct{}{}
				if strings.HasPrefix(ss[0], ".") {
					//ignore local bind
					return errors.New("TO DO")
				} else {
					hConfig.Binds = append(hConfig.Binds, cVolumes[ss[0]]+":"+ss[1])
					/*
						hConfig.Mounts = append(hConfig.Mounts,
							mount.Mount{Target: ss[1],
								Source:   cVolumes[ss[0]],
								Type:     "volume",
								ReadOnly: false,
							})
					*/
				}
			default:
				//ignore "target", "source:target:ro"
				return errors.New("TO DO")
			}
		}
		if service.StandbyPort != "" {
			cConfig.Labels["traefik.enable"] = "true"
			cConfig.Labels["traefik.docker.network"] = "web_nw"
			cConfig.Labels["traefik.http.routers."+cName+".rule"] = "Host(`web.negineri.com`) && PathPrefix(`/~" + userConfig.UserID + "/" + userConfig.Route + "`)"
			cConfig.Labels["traefik.http.routers."+cName+".entrypoints"] = "websecure"
			cConfig.Labels["traefik.http.routers."+cName+".tls.certresolver"] = "myresolver"
			cConfig.Labels["traefik.http.services."+cName+".loadbalancer.server.port"] = service.StandbyPort
			nConfig.EndpointsConfig["web_nw"] = nil
		}
		if _, err := cli.ContainerCreate(ctx, &cConfig, &hConfig, nil, cName); err != nil {
			return err
		}
		if err := cli.NetworkConnect(ctx, stackName+"_default", cName, nConfig.EndpointsConfig[stackName+"_default"]); err != nil {
			return err
		}
		if service.StandbyPort != "" {
			if err := cli.NetworkConnect(ctx, "web_nw", cName, nConfig.EndpointsConfig["web_nw"]); err != nil {
				return err
			}
		}

		if err := cli.ContainerStart(ctx, cName, types.ContainerStartOptions{}); err != nil {
			return err
		}
	}
	chownXID := xid.New()
	cConfig := container.Config{AttachStdout: false,
		AttachStderr: false,
		Volumes:      make(map[string]struct{})}
	hConfig := container.HostConfig{
		LogConfig: container.LogConfig{Type: "json-file"}, AutoRemove: true}
	cConfig.Image = "alpine:latest"
	chownCmd := []string{"chown", "-R", userConfig.UID, "/mnt"}
	cConfig.Cmd = append(cConfig.Cmd, chownCmd...)
	if _, _, err := cli.ImageInspectWithRaw(ctx, cConfig.Image); err != nil {
		rc, err := cli.ImagePull(ctx, "docker.io/library/"+cConfig.Image, types.ImagePullOptions{})
		io.Copy(os.Stdout, rc)
		defer rc.Close()
		if err != nil {
			return nil
		}
	}
	for vName, _ := range config.Chown {

		cConfig.Volumes["/mnt/"+vName] = struct{}{}
		hConfig.Binds = append(hConfig.Binds, cVolumes[vName]+":/mnt/"+vName)
		/*
			hConfig.Mounts = append(hConfig.Mounts,
				mount.Mount{Target: ss[1],
					Source:   cVolumes[ss[0]],
					Type:     "volume",
					ReadOnly: false,
				})
		*/

	}
	if _, err := cli.ContainerCreate(ctx, &cConfig, &hConfig, nil, chownXID.String()); err != nil {
		return err
	}
	// TODO need sleep
	if err := cli.ContainerStart(ctx, chownXID.String(), types.ContainerStartOptions{}); err != nil {
		return err
	}
	fmt.Printf("chown %s\n", chownXID.String())
	return nil
}
