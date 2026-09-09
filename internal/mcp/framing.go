package mcp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// NewStdioFraming wraps a stdio pair so newline-delimited JSON-RPC and
// LSP-style Content-Length frames both work. The first inbound message
// selects the dialect; replies use the same framing.
func NewStdioFraming(in io.ReadCloser, out io.WriteCloser) (io.ReadCloser, io.WriteCloser) {
	state := &framingState{}
	return &framingReader{r: bufio.NewReader(in), closer: in, state: state}, &framingWriter{w: out, closer: out, state: state}
}

type framingMode int

const (
	modeUnknown framingMode = iota
	modeNDJSON
	modeHeaders
)

type framingState struct {
	mu   sync.Mutex
	mode framingMode
}

func (s *framingState) set(mode framingMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == modeUnknown {
		s.mode = mode
	}
}

func (s *framingState) get() framingMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

type framingReader struct {
	r      *bufio.Reader
	closer io.Closer
	state  *framingState
	buf    bytes.Buffer
}

func (f *framingReader) Read(p []byte) (int, error) {
	if f.buf.Len() == 0 {
		if err := f.fill(); err != nil && f.buf.Len() == 0 {
			return 0, err
		}
	}
	return f.buf.Read(p)
}

func (f *framingReader) Close() error {
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
}

func (f *framingReader) fill() error {
	if err := f.detect(); err != nil {
		return err
	}
	if f.state.get() == modeHeaders {
		return f.readHeaderFrame()
	}
	line, err := f.r.ReadBytes('\n')
	if len(line) > 0 {
		f.buf.Write(line)
		if line[len(line)-1] != '\n' {
			f.buf.WriteByte('\n')
		}
	}
	return err
}

func (f *framingReader) detect() error {
	if f.state.get() != modeUnknown {
		return nil
	}
	for {
		b, err := f.r.Peek(1)
		if err != nil {
			return err
		}
		if b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t' {
			_, _ = f.r.ReadByte()
			continue
		}
		if b[0] == '{' {
			f.state.set(modeNDJSON)
			return nil
		}
		f.state.set(modeHeaders)
		return nil
	}
}

func (f *framingReader) readHeaderFrame() error {
	contentLength := -1
	for {
		line, err := f.r.ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		idx := strings.IndexByte(trimmed, ':')
		if idx < 0 {
			return fmt.Errorf("invalid MCP header line %q", trimmed)
		}
		name := strings.TrimSpace(trimmed[:idx])
		value := strings.TrimSpace(trimmed[idx+1:])
		if strings.EqualFold(name, "Content-Length") {
			n, convErr := strconv.Atoi(value)
			if convErr != nil || n < 0 {
				return fmt.Errorf("invalid Content-Length header")
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return fmt.Errorf("Content-Length MCP frame missing length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(f.r, body); err != nil {
		return err
	}
	f.buf.Write(body)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		f.buf.WriteByte('\n')
	}
	return nil
}

type framingWriter struct {
	w      io.Writer
	closer io.Closer
	state  *framingState
	mu     sync.Mutex
	buf    bytes.Buffer
}

func (f *framingWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf.Write(p)
	for {
		data := f.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			return len(p), nil
		}
		line := append([]byte(nil), data[:idx]...)
		f.buf.Next(idx + 1)
		if err := f.writeMessage(line); err != nil {
			return len(p), err
		}
	}
}

func (f *framingWriter) writeMessage(line []byte) error {
	line = bytes.TrimRight(line, "\r")
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	if f.state.get() == modeHeaders {
		if _, err := io.WriteString(f.w, fmt.Sprintf("Content-Length: %d\r\n\r\n", len(line))); err != nil {
			return err
		}
		_, err := f.w.Write(line)
		return err
	}
	_, err := f.w.Write(append(line, '\n'))
	return err
}

func (f *framingWriter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.buf.Len() > 0 {
		if err := f.writeMessage(f.buf.Bytes()); err != nil {
			return err
		}
		f.buf.Reset()
	}
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
}
