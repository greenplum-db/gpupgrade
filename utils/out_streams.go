package utils

import "io"

type OutStreams interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

type OutStreamsCloser interface {
	OutStreams
	Close() error
}
