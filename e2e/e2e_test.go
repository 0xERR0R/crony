//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
