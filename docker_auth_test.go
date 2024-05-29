package testcontainers

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go/internal/core"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestBuildContainerFromDockerfile(t *testing.T) {
	ctx := context.Background()
	req := Request{
		FromDockerfile: FromDockerfile{
			Context: "./testdata",
		},
		AlwaysPullImage: true, // make sure the authentication takes place
		ExposedPorts:    []string{"6379/tcp"},
		WaitingFor:      wait.ForLog("Ready to accept connections"),
		Started:         true,
	}

	redisC, err := New(ctx, req)
	require.NoError(t, err)
	TerminateContainerOnEnd(t, ctx, redisC)
}

// removeImageFromLocalCache removes the image from the local cache
func removeImageFromLocalCache(t *testing.T, img string) {
	ctx := context.Background()

	testcontainersClient, err := core.NewClient(ctx, client.WithVersion(daemonMaxVersion))
	if err != nil {
		t.Log("could not create client to cleanup registry: ", err)
	}
	defer testcontainersClient.Close()

	_, err = testcontainersClient.ImageRemove(ctx, img, image.RemoveOptions{
		Force:         true,
		PruneChildren: true,
	})
	if err != nil {
		t.Logf("could not remove image %s: %v\n", img, err)
	}
}

func TestBuildContainerFromDockerfileWithDockerAuthConfig(t *testing.T) {
	// using the same credentials as in the Docker Registry
	base64 := "dGVzdHVzZXI6dGVzdHBhc3N3b3Jk" // testuser:testpassword
	t.Setenv("DOCKER_AUTH_CONFIG", `{
		"auths": {
				"localhost:5001": { "username": "testuser", "password": "testpassword", "auth": "`+base64+`" }
		},
		"credsStore": "desktop"
	}`)

	prepareLocalRegistryWithAuth(t)

	ctx := context.Background()

	req := Request{
		FromDockerfile: FromDockerfile{
			Context:    "./testdata",
			Dockerfile: "auth.Dockerfile",
		},
		AlwaysPullImage: true, // make sure the authentication takes place
		ExposedPorts:    []string{"6379/tcp"},
		WaitingFor:      wait.ForLog("Ready to accept connections"),
		Started:         true,
	}

	redisC, err := New(ctx, req)
	require.NoError(t, err)
	TerminateContainerOnEnd(t, ctx, redisC)
}

func TestBuildContainerFromDockerfileShouldFailWithWrongDockerAuthConfig(t *testing.T) {
	// using different credentials than in the Docker Registry
	base64 := "Zm9vOmJhcg==" // foo:bar
	t.Setenv("DOCKER_AUTH_CONFIG", `{
		"auths": {
			"localhost:5001": { "username": "foo", "password": "bar", "auth": "`+base64+`" }
		},
		"credsStore": "desktop"
	}`)

	prepareLocalRegistryWithAuth(t)

	ctx := context.Background()

	req := Request{
		FromDockerfile: FromDockerfile{
			Context:    "./testdata",
			Dockerfile: "auth.Dockerfile",
		},
		AlwaysPullImage: true, // make sure the authentication takes place
		ExposedPorts:    []string{"6379/tcp"},
		WaitingFor:      wait.ForLog("Ready to accept connections"),
		Started:         true,
	}

	redisC, err := New(ctx, req)
	require.Error(t, err)
	TerminateContainerOnEnd(t, ctx, redisC)
}

func TestCreateContainerFromPrivateRegistry(t *testing.T) {
	// using the same credentials as in the Docker Registry
	base64 := "dGVzdHVzZXI6dGVzdHBhc3N3b3Jk" // testuser:testpassword
	t.Setenv("DOCKER_AUTH_CONFIG", `{
		"auths": {
				"localhost:5001": { "username": "testuser", "password": "testpassword", "auth": "`+base64+`" }
		},
		"credsStore": "desktop"
	}`)

	prepareLocalRegistryWithAuth(t)

	ctx := context.Background()
	req := Request{
		Image:           "localhost:5001/redis:5.0-alpine",
		AlwaysPullImage: true, // make sure the authentication takes place
		ExposedPorts:    []string{"6379/tcp"},
		WaitingFor:      wait.ForLog("Ready to accept connections"),
		Started:         true,
	}

	redisContainer, err := New(ctx, req)
	require.NoError(t, err)
	TerminateContainerOnEnd(t, ctx, redisContainer)
}

func prepareLocalRegistryWithAuth(t *testing.T) {
	ctx := context.Background()
	wd, err := os.Getwd()
	require.NoError(t, err)

	req := Request{
		Image:        "registry:2",
		ExposedPorts: []string{"5001:5000/tcp"},
		Env: map[string]string{
			"REGISTRY_AUTH":                             "htpasswd",
			"REGISTRY_AUTH_HTPASSWD_REALM":              "Registry",
			"REGISTRY_AUTH_HTPASSWD_PATH":               "/auth/htpasswd",
			"REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY": "/data",
		},
		Files: []ContainerFile{
			{
				HostFilePath:      fmt.Sprintf("%s/testdata/auth", wd),
				ContainerFilePath: "/auth",
			},
			{
				HostFilePath:      fmt.Sprintf("%s/testdata/data", wd),
				ContainerFilePath: "/data",
			},
		},
		WaitingFor: wait.ForExposedPort(),
		Started:    true,
	}

	registryC, err := New(ctx, req)
	require.NoError(t, err)

	t.Cleanup(func() {
		removeImageFromLocalCache(t, "localhost:5001/redis:5.0-alpine")
	})
	t.Cleanup(func() {
		require.NoError(t, registryC.Terminate(context.Background()))
	})

	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
}
