//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const defaultCronyImage = "crony-e2e:latest"

var cronyImage string

func TestMain(m *testing.M) {
	cronyImage = os.Getenv("CRONY_IMAGE")
	if cronyImage == "" {
		cronyImage = defaultCronyImage
	}
	fmt.Fprintf(os.Stderr, "e2e: using crony image %q\n", cronyImage)
	os.Exit(m.Run())
}

func TestJob_RunsOnSchedule_Success(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	startJobContainer(t, s, jobOpts{
		name:     "crony-e2e-job-success",
		cmd:      []string{"sh", "-c", "echo hello; exit 0"},
		schedule: "@every 2s",
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-job-success", "success": "true"}, 1)

	dur, ok := scrapeMetric(t, s.metricsURL, "crony_last_duration_sec",
		map[string]string{"container_name": "crony-e2e-job-success", "success": "true"})
	require.True(t, ok, "duration gauge missing")
	require.GreaterOrEqual(t, dur, 0.0)

	ts, ok := scrapeMetric(t, s.metricsURL, "crony_last_execution_ts",
		map[string]string{"container_name": "crony-e2e-job-success", "success": "true"})
	require.True(t, ok, "last execution gauge missing")
	require.Greater(t, ts, 0.0)
}

func TestJob_RunsOnSchedule_Failure(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	startJobContainer(t, s, jobOpts{
		name:     "crony-e2e-job-failure",
		cmd:      []string{"sh", "-c", "echo boom >&2; exit 1"},
		schedule: "@every 2s",
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-job-failure", "success": "false"}, 1)
}

func TestJob_SkipIfStillRunning(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	startJobContainer(t, s, jobOpts{
		name:     "crony-e2e-job-longrun",
		cmd:      []string{"sh", "-c", "sleep 10"},
		schedule: "@every 2s",
	})

	// Wait for the first execution to finish (counter increments only after job completes).
	require.Eventually(t, func() bool {
		_, ok := scrapeMetric(t, s.metricsURL, "crony_last_execution_ts",
			map[string]string{"container_name": "crony-e2e-job-longrun", "success": "true"})
		return ok
	}, 30*time.Second, 250*time.Millisecond, "first execution never finished")

	// The first 10s job finished. In the next 5s, SkipIfStillRunning should prevent
	// overlapping executions; at most one more can complete.
	time.Sleep(5 * time.Second)
	v, ok := scrapeMetric(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-job-longrun", "success": "true"})
	require.True(t, ok)
	require.LessOrEqual(t, v, 2.0,
		"expected SkipIfStillRunning to prevent overlapping executions, got %v executions", v)
}

// TestStartupRegistration verifies a labeled container created BEFORE crony starts
// is picked up by registerContainers() rather than the event listener.
func TestStartupRegistration(t *testing.T) {
	ctx := context.Background()

	net, err := network.New(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = net.Remove(context.Background()) })

	_ = startMailpit(t, ctx, net)
	_ = startMockserver(t, ctx, net)

	// Create the labeled job BEFORE starting crony.
	jobReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    jobImage,
			Name:     "crony-e2e-startup-job",
			Cmd:      []string{"sh", "-c", "echo hi; exit 0"},
			Labels:   map[string]string{"crony.schedule": "@every 2s"},
			Networks: []string{net.Name},
		},
		Started: false,
	}
	jobC, err := testcontainers.GenericContainer(ctx, jobReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jobC.Terminate(context.Background()) })

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
				"MAIL_POLICY": "never",
				"HC_BASE_URL": "http://" + mockserverNetworkAlias + ":1080/",
				"LOG_LEVEL":   "debug",
			},
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.Binds = append(hc.Binds, dockerSocketPath+":"+dockerSocketPath)
			},
			WaitingFor: wait.ForHTTP("/metrics").WithPort(cronyMetricsPort).WithStartupTimeout(20 * time.Second),
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
	metricsURL := "http://" + host + ":" + port.Port() + "/metrics"

	eventuallyMetricAtLeast(t, metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-startup-job", "success": "true"}, 1)
}

func TestDynamicRegistration_CreateAfterStart(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	// Crony is already running. Create a labeled job and verify it gets picked up.
	startJobContainer(t, s, jobOpts{
		name:     "crony-e2e-dynamic-create",
		cmd:      []string{"sh", "-c", "echo hi; exit 0"},
		schedule: "@every 2s",
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-dynamic-create", "success": "true"}, 1)
}

func TestDynamicRegistration_DestroyRemovesJob(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	jobC := startJobContainer(t, s, jobOpts{
		name:     "crony-e2e-dynamic-destroy",
		cmd:      []string{"sh", "-c", "echo hi; exit 0"},
		schedule: "@every 2s",
	})

	// Wait until at least one execution has happened.
	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-dynamic-destroy", "success": "true"}, 1)

	// Destroy the container.
	require.NoError(t, jobC.Terminate(context.Background()))

	// Capture current count, wait, and assert it didn't grow.
	before, ok := scrapeMetric(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-dynamic-destroy", "success": "true"})
	require.True(t, ok)

	time.Sleep(5 * time.Second)

	after, _ := scrapeMetric(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-dynamic-destroy", "success": "true"})
	require.Equal(t, before, after, "executions kept happening after container was destroyed")
}
