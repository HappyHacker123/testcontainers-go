package gcloud

import (
	"context"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// RunBigQueryContainer creates an instance of the GCloud container type for BigQuery.
// The URI will always use http:// as the protocol.
func RunBigQueryContainer(ctx context.Context, opts ...testcontainers.RequestCustomizer) (*Container, error) {
	req := testcontainers.Request{
		Image:        "ghcr.io/goccy/bigquery-emulator:0.4.3",
		ExposedPorts: []string{"9050/tcp", "9060/tcp"},
		WaitingFor:   wait.ForHTTP("/discovery/v1/apis/bigquery/v2/rest").WithPort("9050/tcp").WithStartupTimeout(time.Second * 5),
		Started:      true,
	}

	settings, err := applyOptions(&req, opts)
	if err != nil {
		return nil, err
	}

	req.Cmd = []string{"--project", settings.ProjectID}

	ctr, err := testcontainers.New(ctx, req)
	if err != nil {
		return nil, err
	}

	spannerContainer, err := newGCloudContainer(ctx, 9050, ctr, settings)
	if err != nil {
		return nil, err
	}

	// always prepend http:// to the URI
	spannerContainer.URI = "http://" + spannerContainer.URI

	return spannerContainer, nil
}
