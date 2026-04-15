//go:build e2e

package e2e

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

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

func TestMail_PolicyNever(t *testing.T) {
	s := setupCronyStack(t, stackOptions{mailPolicy: "never"})

	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-mail-never-ok",
		cmd:  []string{"sh", "-c", "echo ok; exit 0"},
	})
	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-mail-never-fail",
		cmd:  []string{"sh", "-c", "echo fail; exit 1"},
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-mail-never-ok", "success": "true"}, 1)
	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-mail-never-fail", "success": "false"}, 1)

	require.Empty(t, s.mailpit.messages(t), "no mail expected with policy=never")
}

func TestMail_PolicyAlways(t *testing.T) {
	s := setupCronyStack(t, stackOptions{mailPolicy: "always"})

	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-mail-always",
		cmd:  []string{"sh", "-c", "echo greetings; exit 0"},
	})

	require.Eventually(t, func() bool {
		return len(s.mailpit.messages(t)) >= 1
	}, 30*time.Second, 500*time.Millisecond, "no mail received with policy=always")

	msgs := s.mailpit.messages(t)
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].Subject, "[SUCCESS]")
	require.Contains(t, msgs[0].Subject, "crony-e2e-mail-always")

	detail := s.mailpit.messageDetail(t, msgs[0].ID)
	body := detail.HTML
	require.Contains(t, body, "crony-e2e-mail-always")
	require.Contains(t, body, "greetings")
}

func TestMail_PolicyOnError_JobSuccess(t *testing.T) {
	s := setupCronyStack(t, stackOptions{mailPolicy: "onerror"})

	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-mail-onerror-ok",
		cmd:  []string{"sh", "-c", "echo ok; exit 0"},
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-mail-onerror-ok", "success": "true"}, 1)

	time.Sleep(2 * time.Second)
	require.Empty(t, s.mailpit.messages(t), "no mail expected for successful job under policy=onerror")
}

func TestMail_PolicyOnError_JobFailure(t *testing.T) {
	s := setupCronyStack(t, stackOptions{mailPolicy: "onerror"})

	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-mail-onerror-fail",
		cmd:  []string{"sh", "-c", "echo boom >&2; exit 1"},
	})

	require.Eventually(t, func() bool {
		return len(s.mailpit.messages(t)) >= 1
	}, 30*time.Second, 500*time.Millisecond, "no mail received for failing job")

	msgs := s.mailpit.messages(t)
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].Subject, "[FAIL]")
	detail := s.mailpit.messageDetail(t, msgs[0].ID)
	require.Contains(t, detail.HTML, "boom")
}

func TestMail_ContainerLabelOverride(t *testing.T) {
	s := setupCronyStack(t, stackOptions{mailPolicy: "never"})

	startJobContainer(t, s, jobOpts{
		name:       "crony-e2e-mail-label-override",
		cmd:        []string{"sh", "-c", "echo override; exit 0"},
		mailPolicy: "always",
	})

	require.Eventually(t, func() bool {
		return len(s.mailpit.messages(t)) >= 1
	}, 30*time.Second, 500*time.Millisecond, "container-level mail policy override didn't take effect")

	msgs := s.mailpit.messages(t)
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].Subject, "[SUCCESS]")
}

func TestHc_SuccessPingLifecycle(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "11111111-1111-1111-1111-111111111111"
	s.mockserver.expectOK(t, uuid, "OK")

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-success",
		cmd:    []string{"sh", "-c", "echo hello; exit 0"},
		hcUUID: uuid,
	})

	require.Eventually(t, func() bool {
		recs := s.mockserver.recordedRequests(t, uuid)
		var sawStart, sawZero bool
		for _, r := range recs {
			if r.pathMatches("/start") {
				sawStart = true
			}
			if r.pathMatches("/0") {
				sawZero = true
			}
		}
		return sawStart && sawZero
	}, 30*time.Second, 500*time.Millisecond, "did not see start + success ping")

	recs := s.mockserver.recordedRequests(t, uuid)
	var zeroBody string
	for _, r := range recs {
		if r.pathMatches("/0") {
			zeroBody = r.Body
			break
		}
	}
	require.Contains(t, zeroBody, "hello", "success ping body should contain stdout")
}

func TestHc_FailurePing(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "22222222-2222-2222-2222-222222222222"
	s.mockserver.expectOK(t, uuid, "OK")

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-failure",
		cmd:    []string{"sh", "-c", "echo boom >&2; exit 1"},
		hcUUID: uuid,
	})

	require.Eventually(t, func() bool {
		for _, r := range s.mockserver.recordedRequests(t, uuid) {
			if r.pathMatches("/1") {
				return true
			}
		}
		return false
	}, 30*time.Second, 500*time.Millisecond, "did not see failure ping")
}

func TestHc_NoUuid_NoPings(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-hc-none",
		cmd:  []string{"sh", "-c", "echo hello; exit 0"},
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-hc-none", "success": "true"}, 1)

	// No UUID was set on the job, so mockserver should have recorded nothing
	// under a placeholder UUID.
	recs := s.mockserver.recordedRequests(t, "00000000-0000-0000-0000-000000000000")
	require.Empty(t, recs, "no hc.io requests expected when no UUID label is set")
}

func TestHc_ServerNotFound(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "33333333-3333-3333-3333-333333333333"
	s.mockserver.expectOK(t, uuid, "OK (not found)")

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-notfound",
		cmd:    []string{"sh", "-c", "echo hi; exit 0"},
		hcUUID: uuid,
	})

	// Job still completes successfully.
	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-hc-notfound", "success": "true"}, 1)

	// Crony logs an error mentioning the UUID.
	require.Eventually(t, func() bool {
		logs := s.cronyLogs(t)
		return strings.Contains(logs, uuid) && strings.Contains(logs, "could not find a check")
	}, 15*time.Second, 500*time.Millisecond, "expected hc not-found error log mentioning UUID")
}

func TestHc_RateLimited(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "44444444-4444-4444-4444-444444444444"
	s.mockserver.expectOK(t, uuid, "OK (rate limited)")

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-ratelimited",
		cmd:    []string{"sh", "-c", "echo hi; exit 0"},
		hcUUID: uuid,
	})

	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-hc-ratelimited", "success": "true"}, 1)

	require.Eventually(t, func() bool {
		return strings.Contains(s.cronyLogs(t), "pinged too frequently")
	}, 15*time.Second, 500*time.Millisecond, "expected rate-limited error log")
}

func TestHc_RetryThenSucceed(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "55555555-5555-5555-5555-555555555555"
	// First two requests drop the connection, third (and beyond) the catch-all OK handles.
	s.mockserver.expectDropConnectionTimes(t, uuid, 2)
	s.mockserver.expectOK(t, uuid, "OK")

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-retry",
		cmd:    []string{"sh", "-c", "echo hi; exit 0"},
		hcUUID: uuid,
	})

	// Expect at least 6 total requests: start ping retries 2+1, end ping retries 2+1.
	require.Eventually(t, func() bool {
		return len(s.mockserver.recordedRequests(t, uuid)) >= 6
	}, 30*time.Second, 500*time.Millisecond, "did not see retried then succeeded requests")
}

func TestHc_GiveUpAfter3(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	uuid := "66666666-6666-6666-6666-666666666666"
	s.mockserver.expectDropConnectionAlways(t, uuid)

	startJobContainer(t, s, jobOpts{
		name:   "crony-e2e-hc-giveup",
		cmd:    []string{"sh", "-c", "echo hi; exit 0"},
		hcUUID: uuid,
	})

	// Job continues despite hc.io failure.
	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-hc-giveup", "success": "true"}, 1)

	// After both start and end pings give up (3 attempts each), we should see 6 recorded requests.
	require.Eventually(t, func() bool {
		return len(s.mockserver.recordedRequests(t, uuid)) >= 6
	}, 30*time.Second, 500*time.Millisecond, "expected 6 dropped attempts (3 for start, 3 for end)")

	recs := s.mockserver.recordedRequests(t, uuid)
	require.LessOrEqual(t, len(recs), 6, "expected at most 6 attempts, got more")
}

func TestMetrics_Endpoint(t *testing.T) {
	s := setupCronyStack(t, stackOptions{})

	// Run a job once so all metric families have at least one sample.
	startJobContainer(t, s, jobOpts{
		name: "crony-e2e-metrics",
		cmd:  []string{"sh", "-c", "echo hi; exit 0"},
	})
	eventuallyMetricAtLeast(t, s.metricsURL, "crony_executed_count",
		map[string]string{"container_name": "crony-e2e-metrics", "success": "true"}, 1)

	resp, err := http.Get(s.metricsURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	text := string(body)

	require.Contains(t, text, "crony_executed_count")
	require.Contains(t, text, "crony_last_duration_sec")
	require.Contains(t, text, "crony_last_execution_ts")

	require.Contains(t, text, "# TYPE crony_executed_count counter")
	require.Contains(t, text, "# TYPE crony_last_duration_sec gauge")
	require.Contains(t, text, "# TYPE crony_last_execution_ts gauge")
}
