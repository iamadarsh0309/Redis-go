package persistence

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"go-redis/internal/protocol"
)

func cmd(parts ...string) *protocol.Value {
	arr := make([]protocol.Value, len(parts))
	for i, p := range parts {
		arr[i] = protocol.Value{Type: protocol.Bulk, Bulk: p}
	}
	return &protocol.Value{Type: protocol.Array, Array: arr}
}

func readAOFCommands(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := protocol.NewParser(f)
	var out [][]string
	for {
		v, err := p.ReadValue()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		args := make([]string, len(v.Array))
		for i, a := range v.Array {
			args[i] = a.Bulk
		}
		out = append(out, args)
	}
	return out
}

func TestAOFAppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.aof")
	a, err := New(path, No)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if err := a.Append(cmd("SET", "k", "v")); err != nil {
		t.Fatal(err)
	}
	if err := a.Append(cmd("SET", "k2", "v2")); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}

	var seen [][]string
	if err := a.Replay(func(v *protocol.Value) {
		args := make([]string, len(v.Array))
		for i, a := range v.Array {
			args[i] = a.Bulk
		}
		seen = append(seen, args)
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0][1] != "k" || seen[1][1] != "k2" {
		t.Fatalf("replay saw %v", seen)
	}
}

func TestCompactRemovesRedundancy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.aof")
	a, err := New(path, No)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	for i := 0; i < 100; i++ {
		if err := a.Append(cmd("SET", "k", "v")); err != nil {
			t.Fatal(err)
		}
	}
	a.Flush()
	before := fileSize(t, path)

	err = a.Compact(func() []*protocol.Value {
		return []*protocol.Value{cmd("SET", "k", "v")}
	})
	if err != nil {
		t.Fatal(err)
	}
	after := fileSize(t, path)

	if after >= before {
		t.Fatalf("compact did not shrink file: before=%d after=%d", before, after)
	}
	cmds := readAOFCommands(t, path)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command after compact, got %d", len(cmds))
	}
	if cmds[0][0] != "SET" || cmds[0][1] != "k" || cmds[0][2] != "v" {
		t.Fatalf("compact wrote wrong command: %v", cmds[0])
	}
}

func TestCompactPreservesConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.aof")
	a, err := New(path, No)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	var concurrentDone atomic.Bool

	provider := func() []*protocol.Value {
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_ = a.Append(cmd("SET", "concurrent", "v"))
			}(i)
		}
		wg.Wait()
		concurrentDone.Store(true)
		return []*protocol.Value{cmd("SET", "snapshot", "yes")}
	}

	if err := a.Compact(provider); err != nil {
		t.Fatal(err)
	}
	if !concurrentDone.Load() {
		t.Fatal("concurrent writers did not complete")
	}

	cmds := readAOFCommands(t, path)
	var snapshotCount, concurrentCount int
	for _, c := range cmds {
		if len(c) >= 2 && c[1] == "snapshot" {
			snapshotCount++
		}
		if len(c) >= 2 && c[1] == "concurrent" {
			concurrentCount++
		}
	}
	if snapshotCount != 1 || concurrentCount != 20 {
		t.Fatalf("post-compact counts: snapshot=%d concurrent=%d, want 1/20", snapshotCount, concurrentCount)
	}
}

func TestCompactRejectsDoubleStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.aof")
	a, err := New(path, No)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	first := make(chan error, 1)
	gate := make(chan struct{})
	go func() {
		first <- a.Compact(func() []*protocol.Value {
			<-gate
			return nil
		})
	}()

	for i := 0; i < 1000; i++ {
		a.mu.Lock()
		started := a.compacting
		a.mu.Unlock()
		if started {
			break
		}
		runtime.Gosched()
	}

	err2 := a.Compact(func() []*protocol.Value { return nil })
	if err2 == nil {
		t.Fatal("expected second concurrent Compact to fail")
	}
	close(gate)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestRDBRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.rdb")
	snap := []*protocol.Value{
		cmd("SET", "a", "1"),
		cmd("HSET", "h", "f1", "v1", "f2", "v2"),
		cmd("RPUSH", "l", "x", "y", "z"),
		cmd("PEXPIREAT", "a", "1234567890"),
	}
	if err := WriteRDB(path, snap); err != nil {
		t.Fatal(err)
	}
	var seen [][]string
	if err := LoadRDB(path, func(v *protocol.Value) {
		args := make([]string, len(v.Array))
		for i, a := range v.Array {
			args[i] = a.Bulk
		}
		seen = append(seen, args)
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(snap) {
		t.Fatalf("got %d commands back, want %d", len(seen), len(snap))
	}
	if seen[0][0] != "SET" || seen[1][0] != "HSET" || seen[2][0] != "RPUSH" || seen[3][0] != "PEXPIREAT" {
		t.Fatalf("wrong command order: %v", seen)
	}
}

func TestRDBMissingFileIsNotAnError(t *testing.T) {
	err := LoadRDB(filepath.Join(t.TempDir(), "nope.rdb"), func(*protocol.Value) {})
	if err != nil {
		t.Fatalf("LoadRDB on missing file returned %v", err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}
