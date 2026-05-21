package persistence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"go-redis/internal/protocol"
)

type FSyncMode string

const (
	Always   FSyncMode = "always"
	EverySec FSyncMode = "everysec"
	No       FSyncMode = "no"
)

type Logger interface {
	Append(v *protocol.Value) error
	Flush() error
}

type Compactor interface {
	Compact(provider func() []*protocol.Value) error
}

type AOF struct {
	mu    sync.Mutex
	f     *os.File
	w     *protocol.Writer
	fsync FSyncMode

	compacting bool
	diffBuf    []*protocol.Value
}

func New(path string, fsync FSyncMode) (*AOF, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &AOF{
		f:     f,
		w:     protocol.NewWriter(f),
		fsync: fsync,
	}, nil
}

func (a *AOF) Append(v *protocol.Value) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.w.Write(v); err != nil {
		return err
	}
	if a.compacting {
		a.diffBuf = append(a.diffBuf, cloneValue(v))
	}
	if a.fsync == Always {
		return a.w.Flush()
	}
	return nil
}

func cloneValue(v *protocol.Value) *protocol.Value {
	if v == nil {
		return nil
	}
	out := &protocol.Value{
		Type: v.Type,
		Bulk: v.Bulk,
		Str:  v.Str,
		Err:  v.Err,
		Int:  v.Int,
	}
	if len(v.Array) > 0 {
		out.Array = make([]protocol.Value, len(v.Array))
		for i := range v.Array {
			out.Array[i] = *cloneValue(&v.Array[i])
		}
	}
	return out
}

func (a *AOF) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.w.Flush()
}

func (a *AOF) Replay(apply func(*protocol.Value)) error {
	if _, err := a.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	p := protocol.NewParser(a.f)
	for {
		v, err := p.ReadValue()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Println("aof replay error:", err)
			break
		}
		apply(v)
	}
	_, err := a.f.Seek(0, io.SeekEnd)
	return err
}

func (a *AOF) StartFlushLoop(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				if err := a.Flush(); err != nil {
					log.Println("aof final flush:", err)
				}
				return
			case <-t.C:
				if err := a.Flush(); err != nil {
					log.Println("aof flush:", err)
				}
			}
		}
	}()
}

func (a *AOF) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.w.Flush(); err != nil {
		return err
	}
	return a.f.Close()
}

func (a *AOF) Compact(provider func() []*protocol.Value) error {
	a.mu.Lock()
	if a.compacting {
		a.mu.Unlock()
		return errors.New("AOF compaction already in progress")
	}
	a.compacting = true
	a.diffBuf = nil
	livePath := a.f.Name()
	a.mu.Unlock()

	snapshot := provider()

	tmpPath := livePath + ".rewrite"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		a.abortCompaction()
		return fmt.Errorf("creating rewrite tempfile: %w", err)
	}
	tw := protocol.NewWriter(tmpFile)
	for _, v := range snapshot {
		if err := tw.Write(v); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			a.abortCompaction()
			return fmt.Errorf("writing snapshot: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		a.abortCompaction()
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	tw2 := protocol.NewWriter(tmpFile)
	for _, v := range a.diffBuf {
		if err := tw2.Write(v); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			a.compacting = false
			a.diffBuf = nil
			return fmt.Errorf("writing diff: %w", err)
		}
	}
	if err := tw2.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		a.compacting = false
		a.diffBuf = nil
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		a.compacting = false
		a.diffBuf = nil
		return err
	}
	tmpFile.Close()

	if err := a.f.Close(); err != nil {
		os.Remove(tmpPath)
		a.compacting = false
		a.diffBuf = nil
		return fmt.Errorf("closing old AOF: %w", err)
	}
	if err := os.Rename(tmpPath, livePath); err != nil {
		if f, oerr := os.OpenFile(livePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644); oerr == nil {
			a.f = f
			a.w = protocol.NewWriter(f)
		}
		a.compacting = false
		a.diffBuf = nil
		return fmt.Errorf("renaming rewrite file: %w", err)
	}
	newFile, err := os.OpenFile(livePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		a.compacting = false
		a.diffBuf = nil
		return fmt.Errorf("reopening new AOF: %w", err)
	}
	a.f = newFile
	a.w = protocol.NewWriter(newFile)
	a.compacting = false
	a.diffBuf = nil
	return nil
}

func (a *AOF) abortCompaction() {
	a.mu.Lock()
	a.compacting = false
	a.diffBuf = nil
	a.mu.Unlock()
}

var _ Compactor = (*AOF)(nil)
