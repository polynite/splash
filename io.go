package main

import (
	"bytes"
	"io"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type ByteCloser struct {
	r *bytes.Reader
}

func (bc ByteCloser) Read(p []byte) (int, error) {
	return bc.r.Read(p)
}

func (bc ByteCloser) Seek(offset int64, whence int) (int64, error) {
	return bc.r.Seek(offset, whence)
}

func (bc ByteCloser) Close() error {
	return nil
}

func NewByteCloser(data []byte) ByteCloser {
	return ByteCloser{bytes.NewReader(data)}
}
