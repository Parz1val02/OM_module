package discovery

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func Discover() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer func() {
		err = cli.Close()
	}()

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers {
		name := "unnamed"
		if len(container.Names) > 0 {
			name = container.Names[0][1:] // Remove leading "/"
		}

		fmt.Printf("Name: %s\n", name)
		fmt.Printf("ID: %s\n", container.ID[:12])
		fmt.Printf("Image: %s\n", container.Image)
		fmt.Printf("Status: %s\n", container.Status)
		fmt.Printf("State: %s\n", container.State)
		fmt.Println("---")
	}
}
