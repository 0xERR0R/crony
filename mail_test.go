package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMailPolicy_StringRoundTrip(t *testing.T) {
	require.Equal(t, "NEVER", Never.String())
	require.Equal(t, "ALWAYS", Always.String())
	require.Equal(t, "ONERROR", OnError.String())
}

func TestMailPolicy_Decode(t *testing.T) {
	cases := []struct {
		input   string
		want    MailPolicy
		wantErr bool
	}{
		{"never", Never, false},
		{"NEVER", Never, false},
		{"Never", Never, false},
		{"always", Always, false},
		{"ALWAYS", Always, false},
		{"onerror", OnError, false},
		{"ONERROR", OnError, false},
		{"OnError", OnError, false},
		{"bogus", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var got MailPolicy
			err := got.Decode(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unknown value")

				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMailConfig_Validate(t *testing.T) {
	t.Run("user and password both set", func(t *testing.T) {
		c := MailConfig{SmtpUser: "u", SmtpPassword: "p"}
		require.NoError(t, c.Validate())
	})
	t.Run("both empty", func(t *testing.T) {
		c := MailConfig{}
		require.NoError(t, c.Validate())
	})
	t.Run("only user", func(t *testing.T) {
		c := MailConfig{SmtpUser: "u"}
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "SMTP_USER and SMTP_PASSWORD")
	})
	t.Run("only password", func(t *testing.T) {
		c := MailConfig{SmtpPassword: "p"}
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "SMTP_USER and SMTP_PASSWORD")
	})
}

func TestCreateTopic(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		topic := createTopic(MailParams{
			ContainerName: "backup",
			ReturnCode:    0,
			Duration:      2 * time.Second,
		})
		require.True(t, strings.HasPrefix(topic, "[SUCCESS]"))
		require.Contains(t, topic, "backup")
		require.Contains(t, topic, "2 seconds")
	})
	t.Run("failure", func(t *testing.T) {
		topic := createTopic(MailParams{
			ContainerName: "backup",
			ReturnCode:    1,
			Duration:      90 * time.Second,
		})
		require.True(t, strings.HasPrefix(topic, "[FAIL]"))
		require.Contains(t, topic, "backup")
		require.Contains(t, topic, "1 minute 30 seconds")
	})
}

func TestMailParams_ShortDuration(t *testing.T) {
	cases := []struct {
		dur  time.Duration
		want string
	}{
		{1500 * time.Millisecond, "1 second"},
		{90 * time.Second, "1 minute 30 seconds"},
		{0, "0 seconds"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			p := MailParams{Duration: tc.dur}
			require.Equal(t, tc.want, p.ShortDuration())
		})
	}
}

func TestNewTemplate_Renders(t *testing.T) {
	tmpl := newTemplate()
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, MailParams{
		ContainerName: "backup",
		ReturnCode:    42,
		Duration:      3 * time.Second,
		StdOut:        "hello stdout",
		StdErr:        "boom stderr",
	})
	require.NoError(t, err)
	rendered := buf.String()
	require.Contains(t, rendered, "backup")
	require.Contains(t, rendered, "42")
	require.Contains(t, rendered, "hello stdout")
	require.Contains(t, rendered, "boom stderr")
	require.Contains(t, rendered, "3 seconds")
}
