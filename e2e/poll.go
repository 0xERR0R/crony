//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultEventually = 20 * time.Second
	defaultTick       = 250 * time.Millisecond
)

// scrapeMetric fetches the prometheus metrics endpoint and returns the value of the
// first sample matching metricName whose label set contains all of the given labels.
// Returns 0 and false if no match.
func scrapeMetric(t *testing.T, url, metricName string, labels map[string]string) (float64, bool) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, metricName) {
			continue
		}
		// line format: name{label="v",label="v"} value timestamp?
		openBrace := strings.Index(line, "{")
		closeBrace := strings.Index(line, "}")
		if openBrace == -1 || closeBrace == -1 {
			continue
		}
		labelStr := line[openBrace+1 : closeBrace]
		got := parseLabels(labelStr)
		if !labelsMatch(got, labels) {
			continue
		}
		rest := strings.TrimSpace(line[closeBrace+1:])
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, part := range splitLabels(s) {
		eq := strings.Index(part, "=")
		if eq == -1 {
			continue
		}
		key := part[:eq]
		val := strings.Trim(part[eq+1:], `"`)
		out[key] = val
	}
	return out
}

// splitLabels splits a Prometheus label list on commas, respecting quoted values.
func splitLabels(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

func labelsMatch(got, want map[string]string) bool {
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// eventuallyMetricAtLeast polls scrapeMetric until the metric value is >= min, or fails the test.
func eventuallyMetricAtLeast(t *testing.T, url, metricName string, labels map[string]string, min float64) {
	t.Helper()
	assert.Eventually(t, func() bool {
		v, ok := scrapeMetric(t, url, metricName, labels)
		return ok && v >= min
	}, defaultEventually, defaultTick,
		"metric %s with labels %v never reached %v", metricName, labels, min)
}
