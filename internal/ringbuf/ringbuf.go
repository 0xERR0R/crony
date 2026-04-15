// Package ringbuf provides a fixed-size ring buffer that retains the last N
// bytes written to it. It implements io.Writer and is intended for capturing
// the tail of unbounded output streams (e.g. container logs) for display or
// notification purposes.
package ringbuf

import "fmt"

// Buffer retains the last cap(buf) bytes written to it. It is not safe for
// concurrent use.
type Buffer struct {
	buf  []byte
	pos  int
	full bool
}

// New returns a Buffer that retains the last size bytes written to it.
// It panics if size <= 0.
func New(size int) *Buffer {
	if size <= 0 {
		panic(fmt.Sprintf("ringbuf: non-positive size %d", size))
	}

	return &Buffer{buf: make([]byte, size)}
}

// Write appends p to the buffer, discarding the oldest bytes as needed to
// stay within the configured size. It always returns len(p), nil.
func (b *Buffer) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	size := len(b.buf)
	if n >= size {
		copy(b.buf, p[n-size:])
		b.pos = 0
		b.full = true

		return n, nil
	}

	written := copy(b.buf[b.pos:], p)
	if written < n {
		copy(b.buf, p[written:])
	}

	b.pos = (b.pos + n) % size
	if written < n || b.pos == 0 {
		b.full = true
	}

	return n, nil
}

// Len returns the number of bytes currently buffered.
func (b *Buffer) Len() int {
	if b.full {
		return len(b.buf)
	}

	return b.pos
}

// Bytes returns a freshly-allocated slice containing the buffered bytes in
// logical order (oldest first).
func (b *Buffer) Bytes() []byte {
	if !b.full {
		out := make([]byte, b.pos)
		copy(out, b.buf[:b.pos])

		return out
	}

	out := make([]byte, len(b.buf))
	n := copy(out, b.buf[b.pos:])
	copy(out[n:], b.buf[:b.pos])

	return out
}

// String returns the buffered bytes as a string.
func (b *Buffer) String() string {
	return string(b.Bytes())
}
