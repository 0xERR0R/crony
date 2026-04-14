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
