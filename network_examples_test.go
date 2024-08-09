package testcontainers_test

import (
	"context"
	"fmt"
	"log"

	dockernetwork "github.com/docker/docker/api/types/network"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

func ExampleNew() {
	// createNetwork {
	ctx := context.Background()

	net, err := testcontainers.NewNetwork(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			log.Fatalf("failed to remove network: %s", err)
		}
	}()
	// }

	fmt.Println(net.ID != "")
	fmt.Println(net.Driver)

	// Output:
	// true
	// bridge
}

func ExampleNew_withOptions() {
	// newNetworkWithOptions {
	ctx := context.Background()

	// dockernetwork is the alias used for github.com/docker/docker/api/types/network
	ipamConfig := dockernetwork.IPAM{
		Driver: "default",
		Config: []dockernetwork.IPAMConfig{
			{
				Subnet:  "10.1.1.0/24",
				Gateway: "10.1.1.254",
			},
		},
		Options: map[string]string{
			"driver": "host-local",
		},
	}
	net, err := testcontainers.NewNetwork(ctx,
		network.WithIPAM(&ipamConfig),
		network.WithAttachable(),
		network.WithDriver("bridge"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			log.Fatalf("failed to remove network: %s", err)
		}
	}()
	// }

	fmt.Println(net.ID != "")
	fmt.Println(net.Driver)

	// Output:
	// true
	// bridge
}
