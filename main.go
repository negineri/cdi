package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/negineri/cdi/compose"
)

func main() {
	fmt.Println("Hello world!")
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	//cli.ContainerCreate(ctx, &container.Config{}, &container.HostConfig{}, &network.NetworkingConfig{}, "test")
	compose.NewStack(ctx, cli, compose.UserConfig{StackName: "wordpress", UserID: "negineri"})

	for _, container := range containers {
		fmt.Printf("%s %s\n", container.ID[:10], container.Names[0])
	}
	return
}
