package proxy

import (
	"bytes"
	"io"
	"sync"
)

const (
	pool4K   = 4 * 1024
	pool32K  = 32 * 1024
	pool256K = 256 * 1024
	pool1M   = 1024 * 1024
)

var (
	bytes4KPool   = sync.Pool{New: func() interface{} { b := make([]byte, pool4K); return &b }}
	bytes32KPool  = sync.Pool{New: func() interface{} { b := make([]byte, pool32K); return &b }}
	bytes256KPool = sync.Pool{New: func() interface{} { b := make([]byte, pool256K); return &b }}
	bytes1MPool   = sync.Pool{New: func() interface{} { b := make([]byte, pool1M); return &b }}
)

func acquireBytes(size int) []byte {
	switch {
	case size <= pool4K:
		return (*bytes4KPool.Get().(*[]byte))[:0]
	case size <= pool32K:
		return (*bytes32KPool.Get().(*[]byte))[:0]
	case size <= pool256K:
		return (*bytes256KPool.Get().(*[]byte))[:0]
	case size <= pool1M:
		return (*bytes1MPool.Get().(*[]byte))[:0]
	default:
		return make([]byte, 0, size)
	}
}

func releaseBytes(buf []byte) {
	c := cap(buf)
	switch c {
	case pool4K:
		b := buf[:pool4K]
		bytes4KPool.Put(&b)
	case pool32K:
		b := buf[:pool32K]
		bytes32KPool.Put(&b)
	case pool256K:
		b := buf[:pool256K]
		bytes256KPool.Put(&b)
	case pool1M:
		b := buf[:pool1M]
		bytes1MPool.Put(&b)
	default:
		// 大块内存不入池，避免长期占用。
	}
}

func growBytes(buf []byte, want int) []byte {
	if cap(buf)-len(buf) >= want {
		return buf
	}
	newCap := cap(buf) * 2
	if newCap < len(buf)+want {
		newCap = len(buf) + want
	}
	newBuf := make([]byte, len(buf), newCap)
	copy(newBuf, buf)
	releaseBytes(buf)
	return newBuf
}

func readAllPooled(reader io.Reader, contentLength int64) ([]byte, error) {
	hint := pool4K
	if contentLength > 0 && contentLength < pool1M {
		hint = int(contentLength)
	}
	out := acquireBytes(hint)
	tmp := acquireBytes(pool32K)
	tmp = tmp[:pool32K]
	defer releaseBytes(tmp)

	for {
		n, err := reader.Read(tmp)
		if n > 0 {
			out = growBytes(out, n)
			out = append(out, tmp[:n]...)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			releaseBytes(out)
			return nil, err
		}
	}
}

type pooledReadCloser struct {
	reader *bytes.Reader
	buf    []byte
	once   sync.Once
}

func newPooledReadCloser(buf []byte) *pooledReadCloser {
	return &pooledReadCloser{
		reader: bytes.NewReader(buf),
		buf:    buf,
	}
}

func (p *pooledReadCloser) Read(b []byte) (int, error) {
	return p.reader.Read(b)
}

func (p *pooledReadCloser) Close() error {
	p.once.Do(func() {
		releaseBytes(p.buf)
	})
	return nil
}
