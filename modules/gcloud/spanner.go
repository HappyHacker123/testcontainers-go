package gcloud

import (
	"context"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// RunSpannerContainer creates an instance of the GCloud container type for Spanner
func RunSpannerContainer(ctx context.Context, opts ...testcontainers.RequestCustomizer) (*Container, error) {
	req := testcontainers.Request{
		Image:        "gcr.io/cloud-spanner-emulator/emulator:1.4.0",
		ExposedPorts: []string{"9010/tcp"},
		WaitingFor:   wait.ForLog("Cloud Spanner emulator running"),
		Started:      true,
	}

	settings, err := applyOptions(&req, opts)
	if err != nil {
		return nil, err
	}

	ctr, err := testcontainers.New(ctx, req)
	if err != nil {
		return nil, err
	}

	return newGCloudContainer(ctx, 9010, ctr, settings)
}
