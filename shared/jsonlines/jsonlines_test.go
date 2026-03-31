package jsonlines_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/shared/jsonlines"
)

// Reader tests

func TestReader_SingleLine(t *testing.T) {
	r := jsonlines.NewReader(strings.NewReader(`{"key":"value"}` + "\n"))
	line, err := r.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"key":"value"}`), line)
}

func testReaderMultipleLines(t *testing.T, input string, expected []string) {
	t.Helper()
	r := jsonlines.NewReader(strings.NewReader(input))
	for _, want := range expected {
		line, err := r.ReadLine()
		require.NoError(t, err)
		assert.Equal(t, []byte(want), line)
	}
	_, err := r.ReadLine()
	assert.Equal(t, io.EOF, err)
}

func TestReader_MultipleLines(t *testing.T) {
	testReaderMultipleLines(t,
		"{\"a\":1}\n{\"b\":2}\n{\"c\":3}\n",
		[]string{`{"a":1}`, `{"b":2}`, `{"c":3}`},
	)
}

func TestReader_EOF(t *testing.T) {
	r := jsonlines.NewReader(strings.NewReader(""))
	_, err := r.ReadLine()
	assert.Equal(t, io.EOF, err)
}

func TestReader_ReturnsCopyNotSlice(t *testing.T) {
	r := jsonlines.NewReader(strings.NewReader("hello\nworld\n"))
	line1, err := r.ReadLine()
	require.NoError(t, err)
	line2, err := r.ReadLine()
	require.NoError(t, err)
	// Mutations to one line must not affect the other
	line1[0] = 'X'
	assert.Equal(t, []byte("world"), line2)
}

func TestReader_LargeLine(t *testing.T) {
	// Verify the reader can handle a line near the 10 MB limit
	large := strings.Repeat("x", 5*1024*1024)
	r := jsonlines.NewReader(strings.NewReader(large + "\n"))
	line, err := r.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, len(large), len(line))
}

// Writer tests

func TestWriter_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	w := jsonlines.NewWriter(&buf)
	err := w.WriteLine([]byte(`{"key":"value"}`))
	require.NoError(t, err)
	assert.Equal(t, "{\"key\":\"value\"}\n", buf.String())
}

func TestWriter_AppendsNewline(t *testing.T) {
	var buf bytes.Buffer
	w := jsonlines.NewWriter(&buf)
	require.NoError(t, w.WriteLine([]byte("line1")))
	require.NoError(t, w.WriteLine([]byte("line2")))
	assert.Equal(t, "line1\nline2\n", buf.String())
}

func TestWriter_ConcurrentWrites(t *testing.T) {
	// Verify that concurrent writes don't interleave (each line ends with \n before the next starts)
	var buf safeBuffer
	w := jsonlines.NewWriter(&buf)

	const workers = 20
	const line = `{"msg":"hello"}`

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, w.WriteLine([]byte(line)))
		}()
	}
	wg.Wait()

	result := buf.String()
	// Each write must be a complete line: split by newline and count
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	assert.Len(t, lines, workers)
	for _, l := range lines {
		assert.Equal(t, line, l)
	}
}

// safeBuffer is a bytes.Buffer with a mutex for concurrent-safe use in tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
