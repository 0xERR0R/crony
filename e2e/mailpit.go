//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	mailpitImage        = "axllent/mailpit:latest"
	mailpitNetworkAlias = "mailpit"
	mailpitSMTPPort     = "1025/tcp"
	mailpitHTTPPort     = "8025/tcp"
)

type mailpitContainer struct {
	testcontainers.Container
	httpBaseURL string
}

type mailpitMessage struct {
	ID      string           `json:"ID"`
	From    mailpitAddress   `json:"From"`
	To      []mailpitAddress `json:"To"`
	Subject string           `json:"Subject"`
	Snippet string           `json:"Snippet"`
}

type mailpitAddress struct {
	Name    string `json:"Name"`
	Address string `json:"Address"`
}

type mailpitMessagesResponse struct {
	Messages []mailpitMessage `json:"messages"`
	Total    int              `json:"total"`
}

type mailpitMessageDetail struct {
	mailpitMessage
	Text string `json:"Text"`
	HTML string `json:"HTML"`
}

func startMailpit(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) *mailpitContainer {
	t.Helper()
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        mailpitImage,
			ExposedPorts: []string{mailpitSMTPPort, mailpitHTTPPort},
			Networks:     []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {mailpitNetworkAlias},
			},
			WaitingFor: wait.ForHTTP("/api/v1/messages").WithPort(mailpitHTTPPort),
		},
		Started: true,
	}
	c, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, mailpitHTTPPort)
	require.NoError(t, err)

	return &mailpitContainer{
		Container:   c,
		httpBaseURL: fmt.Sprintf("http://%s:%s", host, port.Port()),
	}
}

func (m *mailpitContainer) messages(t *testing.T) []mailpitMessage {
	t.Helper()
	resp, err := http.Get(m.httpBaseURL + "/api/v1/messages")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out mailpitMessagesResponse
	require.NoError(t, json.Unmarshal(body, &out))
	return out.Messages
}

func (m *mailpitContainer) messageDetail(t *testing.T, id string) mailpitMessageDetail {
	t.Helper()
	resp, err := http.Get(m.httpBaseURL + "/api/v1/message/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out mailpitMessageDetail
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

