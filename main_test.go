package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func setBaseSMTPEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("MAIL_TO", "to@example.com")
	t.Setenv("MAIL_FROM", "from@example.com")
}

func TestMailConfig_HappyPath(t *testing.T) {
	setBaseSMTPEnv(t)
	t.Setenv("MAIL_POLICY", "always")

	cfg := mailConfig(CronyContainer{})
	require.NotNil(t, cfg)
	require.Equal(t, "smtp.example.com", cfg.SmtpHost)
	require.Equal(t, 587, cfg.SmtpPort)
	require.Equal(t, "to@example.com", cfg.MailTo)
	require.Equal(t, "from@example.com", cfg.MailFrom)
	require.Equal(t, Always, cfg.MailPolicy)
}

func TestMailConfig_MissingRequired_ReturnsNil(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("MAIL_TO", "")
	t.Setenv("MAIL_FROM", "")
	cfg := mailConfig(CronyContainer{})
	require.Nil(t, cfg)
}

func TestMailConfig_ContainerLabelOverridesGlobal(t *testing.T) {
	setBaseSMTPEnv(t)
	t.Setenv("MAIL_POLICY", "never")

	cfg := mailConfig(CronyContainer{MailPolicy: "always"})
	require.NotNil(t, cfg)
	require.Equal(t, Always, cfg.MailPolicy)
}

func TestMailConfig_InvalidContainerLabel_FallsBackToGlobal(t *testing.T) {
	setBaseSMTPEnv(t)
	t.Setenv("MAIL_POLICY", "onerror")

	cfg := mailConfig(CronyContainer{MailPolicy: "garbage"})
	require.NotNil(t, cfg)
	require.Equal(t, OnError, cfg.MailPolicy)
}
