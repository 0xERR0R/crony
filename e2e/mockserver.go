//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	mockserverImage        = "mockserver/mockserver:latest"
	mockserverNetworkAlias = "mockserver"
	mockserverPort         = "1080/tcp"
)

type mockserverContainer struct {
	testcontainers.Container
	httpBaseURL string
}

func startMockserver(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) *mockserverContainer {
	t.Helper()
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        mockserverImage,
			ExposedPorts: []string{mockserverPort},
			Networks:     []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {mockserverNetworkAlias},
			},
			WaitingFor: wait.ForHTTP("/mockserver/status").WithPort(mockserverPort).WithMethod(http.MethodPut),
		},
		Started: true,
	}
	c, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, mockserverPort)
	require.NoError(t, err)

	return &mockserverContainer{
		Container:   c,
		httpBaseURL: fmt.Sprintf("http://%s:%s", host, port.Port()),
	}
}

// expectOK sets up mockserver to respond with the given body for any POST under /{uuid}/...
func (m *mockserverContainer) expectOK(t *testing.T, uuid, body string) {
	t.Helper()
	expectation := map[string]any{
		"httpRequest": map[string]any{
			"method": "POST",
			"path":   "/" + uuid + "/.*",
		},
		"httpResponse": map[string]any{
			"statusCode": 200,
			"body":       body,
		},
	}
	m.putExpectation(t, expectation)
}

// expectDropConnectionTimes makes mockserver close the TCP connection on the first n requests
// matching POST /{uuid}/... — used to simulate transport errors that crony retries on.
func (m *mockserverContainer) expectDropConnectionTimes(t *testing.T, uuid string, n int) {
	t.Helper()
	expectation := map[string]any{
		"httpRequest": map[string]any{
			"method": "POST",
			"path":   "/" + uuid + "/.*",
		},
		"httpError": map[string]any{
			"dropConnection": true,
		},
		"times": map[string]any{
			"remainingTimes": n,
			"unlimited":      false,
		},
	}
	m.putExpectation(t, expectation)
}

// expectDropConnectionAlways makes mockserver close the TCP connection on every matching request.
func (m *mockserverContainer) expectDropConnectionAlways(t *testing.T, uuid string) {
	t.Helper()
	expectation := map[string]any{
		"httpRequest": map[string]any{
			"method": "POST",
			"path":   "/" + uuid + "/.*",
		},
		"httpError": map[string]any{
			"dropConnection": true,
		},
	}
	m.putExpectation(t, expectation)
}

func (m *mockserverContainer) putExpectation(t *testing.T, expectation map[string]any) {
	t.Helper()
	body, err := json.Marshal(expectation)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, m.httpBaseURL+"/mockserver/expectation", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.True(t, resp.StatusCode < 300, "mockserver expectation PUT failed with %d", resp.StatusCode)
}

// recordedRequests retrieves all requests recorded by mockserver matching the given UUID path prefix.
func (m *mockserverContainer) recordedRequests(t *testing.T, uuid string) []recordedRequest {
	t.Helper()
	criteria := map[string]any{
		"path": "/" + uuid + "/.*",
	}
	body, err := json.Marshal(criteria)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, m.httpBaseURL+"/mockserver/retrieve?type=REQUESTS&format=JSON", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var raw []map[string]any
	require.NoError(t, json.Unmarshal(respBody, &raw), "mockserver retrieve returned non-JSON: %s", string(respBody))
	out := make([]recordedRequest, 0, len(raw))
	for _, r := range raw {
		method, _ := r["method"].(string)
		path, _ := r["path"].(string)
		var bodyStr string
		switch b := r["body"].(type) {
		case string:
			// MockServer returns plain string bodies directly as a JSON string
			bodyStr = b
		case map[string]any:
			// MockServer returns structured bodies (e.g. JSON, binary) as an object
			if s, ok := b["string"].(string); ok {
				bodyStr = s
			} else if enc, ok := b["bytes"].(string); ok {
				// MockServer base64-encodes binary / no-content-type bodies
				if decoded, err := base64.StdEncoding.DecodeString(enc); err == nil {
					bodyStr = string(decoded)
				}
			}
		}
		out = append(out, recordedRequest{Method: method, Path: path, Body: bodyStr})
	}
	return out
}

type recordedRequest struct {
	Method string
	Path   string
	Body   string
}

// pathMatches returns true if the recorded request path ends with the given suffix.
func (r recordedRequest) pathMatches(suffix string) bool {
	return strings.HasSuffix(r.Path, suffix)
}
