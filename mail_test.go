package main

import (
	"testing"

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
