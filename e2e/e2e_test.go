//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"

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
