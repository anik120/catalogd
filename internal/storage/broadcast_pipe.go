package storage

import (
	"errors"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"
)

func broadcastPipe(numReaders int) ([]io.Reader, io.WriteCloser) {
	pipes := make([]struct {
		reader *io.PipeReader
		writer *io.PipeWriter
	}, numReaders)

	readers := make([]io.Reader, numReaders)
	writers := make([]*io.PipeWriter, numReaders)
	for i := range numReaders {
		pr, pw := io.Pipe()
		pipes[i] = struct {
			reader *io.PipeReader
			writer *io.PipeWriter
		}{pr, pw}
		readers[i] = pr
		writers[i] = pw
	}

	bw := &broadcastWriter{
		writers: writers,
	}

	return readers, bw
}

type broadcastWriter struct {
	mu      sync.Mutex
	writers []*io.PipeWriter
}

func (bw *broadcastWriter) Write(p []byte) (n int, err error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	var eg errgroup.Group
	writeLen := len(p)

	for _, w := range bw.writers {
		w := w
		eg.Go(func() error {
			n, err := w.Write(p)
			if err != nil {
				return err
			}
			if n != writeLen {
				return io.ErrShortWrite
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		for _, w := range bw.writers {
			_ = w.CloseWithError(err)
		}
		return 0, err
	}
	return writeLen, nil
}

func (bw *broadcastWriter) Close() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	closeErrs := make([]error, len(bw.writers))
	for i, w := range bw.writers {
		closeErrs[i] = w.Close()
	}
	return errors.Join(closeErrs...)
}
