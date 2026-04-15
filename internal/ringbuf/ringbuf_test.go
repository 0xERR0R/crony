package ringbuf

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ io.Writer = (*Buffer)(nil)

func TestNew_PanicsOnNonPositiveSize(t *testing.T) {
	assert.Panics(t, func() { New(0) })
	assert.Panics(t, func() { New(-1) })
}

func TestEmptyBuffer(t *testing.T) {
	b := New(16)
	assert.Equal(t, 0, b.Len())
	assert.Empty(t, b.String())
	assert.Empty(t, b.Bytes())
}

func TestWriteBelowCap(t *testing.T) {
	b := New(16)
	n, err := b.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, b.Len())
	assert.Equal(t, "hello", b.String())
}

func TestMultipleSmallWrites(t *testing.T) {
	b := New(16)
	_, _ = b.Write([]byte("hello "))
	_, _ = b.Write([]byte("world"))
	assert.Equal(t, "hello world", b.String())
	assert.Equal(t, 11, b.Len())
}

func TestWriteExactlyFillsBuffer(t *testing.T) {
	b := New(5)
	n, err := b.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, b.Len())
	assert.Equal(t, "hello", b.String())
}

func TestWrapAroundManySmallWrites(t *testing.T) {
	b := New(5)
	for _, s := range []string{"ab", "cd", "ef", "gh"} {
		_, _ = b.Write([]byte(s))
	}
	// Total written: "abcdefgh" (8 bytes). Last 5 = "defgh".
	assert.Equal(t, 5, b.Len())
	assert.Equal(t, "defgh", b.String())
}

func TestSingleWriteLargerThanCap(t *testing.T) {
	b := New(4)
	input := []byte("abcdefghij")
	n, err := b.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, 4, b.Len())
	assert.Equal(t, "ghij", b.String())
}

func TestWriteAfterWrap(t *testing.T) {
	b := New(5)
	_, _ = b.Write([]byte("abcdef")) // fills, wraps once; contents "bcdef"
	_, _ = b.Write([]byte("gh"))     // contents "defgh"
	assert.Equal(t, "defgh", b.String())
}

func TestEmptyWrite(t *testing.T) {
	b := New(8)
	n, err := b.Write(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = b.Write([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	assert.Equal(t, 0, b.Len())
}

func TestIoCopyKeepsTail(t *testing.T) {
	src := bytes.Repeat([]byte("0123456789"), 100) // 1000 bytes
	b := New(32)
	n, err := io.Copy(b, bytes.NewReader(src))
	require.NoError(t, err)
	assert.Equal(t, int64(len(src)), n)
	assert.Equal(t, 32, b.Len())
	assert.Equal(t, string(src[len(src)-32:]), b.String())
}

func TestBytesReturnsCopy(t *testing.T) {
	b := New(8)
	_, _ = b.Write([]byte("hello"))
	out := b.Bytes()
	out[0] = 'X'
	assert.Equal(t, "hello", b.String())
}
