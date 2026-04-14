package main

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

func TestLogLevelForReturnCode(t *testing.T) {
	require.Equal(t, logrus.DebugLevel, logLevelForReturnCode(0))
	require.Equal(t, logrus.WarnLevel, logLevelForReturnCode(1))
	require.Equal(t, logrus.WarnLevel, logLevelForReturnCode(255))
	require.Equal(t, logrus.WarnLevel, logLevelForReturnCode(-1))
}

func TestSkipLogger_Info(t *testing.T) {
	hook := test.NewGlobal()
	defer hook.Reset()

	skipper := &SkipLogger{containerName: "my-job"}
	skipper.Info("ignored", "args")

	entries := hook.AllEntries()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "my-job")
	require.Contains(t, entries[0].Message, "skipping execution")
	require.Equal(t, logrus.InfoLevel, entries[0].Level)
}
