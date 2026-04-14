//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	cronyMetricsPort  = "8080/tcp"
	cronyNetworkAlias = "crony"
	dockerSocketPath  = "/var/run/docker.sock"
)

type stack struct {
	ctx        context.Context
	net        *testcontainers.DockerNetwork
	mailpit    *mailpitContainer
	mockserver *mockserverContainer
	crony      testcontainers.Container
	metricsURL string
}

type stackOptions struct {
	mailPolicy string // "never", "always", "onerror" — empty means "never"
}

func setupCronyStack(t *testing.T, opts stackOptions) *stack {
	t.Helper()
	ctx := context.Background()

	net, err := network.New(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = net.Remove(context.Background()) })

	mp := startMailpit(t, ctx, net)
	ms := startMockserver(t, ctx, net)

	if opts.mailPolicy == "" {
		opts.mailPolicy = "never"
	}

	cronyReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        cronyImage,
			ExposedPorts: []string{cronyMetricsPort},
			Networks:     []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {cronyNetworkAlias},
			},
			Env: map[string]string{
				"SMTP_HOST":   mailpitNetworkAlias,
				"SMTP_PORT":   "1025",
				"MAIL_TO":     "to@example.com",
				"MAIL_FROM":   "from@example.com",
				"MAIL_POLICY": opts.mailPolicy,
				"HC_BASE_URL": fmt.Sprintf("http://%s:1080/", mockserverNetworkAlias),
				"LOG_LEVEL":   "debug",
			},
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.Binds = append(hc.Binds, dockerSocketPath+":"+dockerSocketPath)
			},
			WaitingFor: wait.ForHTTP("/metrics").
				WithPort(cronyMetricsPort).
				WithStartupTimeout(20 * time.Second),
		},
		Started: true,
	}
	cronyC, err := testcontainers.GenericContainer(ctx, cronyReq)
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			dumpContainerLogs(t, cronyC, "crony")
		}
		_ = cronyC.Terminate(context.Background())
	})

	host, err := cronyC.Host(ctx)
	require.NoError(t, err)
	port, err := cronyC.MappedPort(ctx, cronyMetricsPort)
	require.NoError(t, err)

	return &stack{
		ctx:        ctx,
		net:        net,
		mailpit:    mp,
		mockserver: ms,
		crony:      cronyC,
		metricsURL: fmt.Sprintf("http://%s:%s/metrics", host, port.Port()),
	}
}

func (s *stack) cronyLogs(t *testing.T) string {
	t.Helper()
	r, err := s.crony.Logs(context.Background())
	require.NoError(t, err)
	defer r.Close()
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(b)
}

func dumpContainerLogs(t *testing.T, c testcontainers.Container, name string) {
	t.Helper()
	r, err := c.Logs(context.Background())
	if err != nil {
		t.Logf("could not read %s logs: %v", name, err)
		return
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Logf("could not read %s logs: %v", name, err)
		return
	}
	t.Logf("=== %s logs ===\n%s\n=== end %s logs ===", name, string(b), name)
}
