package persistence

import (
	"errors"
	"fmt"
	"io"
	"os"

	"go-redis/internal/protocol"
)

func WriteRDB(path string, snapshot []*protocol.Value) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("rdb open temp: %w", err)
	}
	w := protocol.NewWriter(f)
	for _, v := range snapshot {
		if err := w.Write(v); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("rdb write: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func LoadRDB(path string, apply func(*protocol.Value)) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	p := protocol.NewParser(f)
	for {
		v, err := p.ReadValue()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("rdb parse: %w", err)
		}
		apply(v)
	}
}
