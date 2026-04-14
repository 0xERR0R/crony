//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const jobImage = "alpine:latest"

type jobOpts struct {
	name        string
	schedule    string            // defaults to "@every 2s" if empty
	mailPolicy  string            // optional crony.mail_policy label
	hcUUID      string            // optional crony.hcio_uuid label
	cmd         []string          // required
	extraLabels map[string]string // optional extras
}

// startJobContainer creates a plain alpine container with crony labels but does NOT start it.
// Crony (listening to Docker events) sees the create event, registers the container, and
// starts it per its schedule. We just need the container to exist with the right labels.
func startJobContainer(t *testing.T, s *stack, opts jobOpts) testcontainers.Container {
	t.Helper()
	require.NotEmpty(t, opts.name, "jobOpts.name is required")
	require.NotEmpty(t, opts.cmd, "jobOpts.cmd is required")

	if opts.schedule == "" {
		opts.schedule = "@every 2s"
	}
	labels := map[string]string{
		"crony.schedule": opts.schedule,
	}
	if opts.mailPolicy != "" {
		labels["crony.mail_policy"] = opts.mailPolicy
	}
	if opts.hcUUID != "" {
		labels["crony.hcio_uuid"] = opts.hcUUID
	}
	for k, v := range opts.extraLabels {
		labels[k] = v
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    jobImage,
			Name:     opts.name,
			Cmd:      opts.cmd,
			Labels:   labels,
			Networks: []string{s.net.Name},
		},
		Started: false,
	}
	c, err := testcontainers.GenericContainer(s.ctx, req)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.Terminate(context.Background())
	})
	return c
}
