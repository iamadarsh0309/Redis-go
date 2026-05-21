package command

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"go-redis/internal/protocol"
)

func cmdInfo(e *Engine, v *protocol.Value) *protocol.Value {
	section := "all"
	if len(v.Array) >= 2 {
		section = strings.ToLower(v.Array[1].Bulk)
	}
	var b strings.Builder
	if section == "server" || section == "all" {
		writeServerInfo(&b, e)
	}
	if section == "memory" || section == "all" {
		writeMemoryInfo(&b, e)
	}
	if section == "stats" || section == "all" {
		writeStatsInfo(&b, e)
	}
	if section == "keyspace" || section == "all" {
		writeKeyspaceInfo(&b, e)
	}
	return bulkReply(b.String())
}

func writeServerInfo(b *strings.Builder, e *Engine) {
	b.WriteString("# Server\r\n")
	fmt.Fprintf(b, "redis_version:redis-go-0.1\r\n")
	fmt.Fprintf(b, "go_version:%s\r\n", runtime.Version())
	fmt.Fprintf(b, "process_id:%d\r\n", processPID())
	fmt.Fprintf(b, "uptime_in_seconds:%d\r\n", int64(time.Since(e.startTime).Seconds()))
	b.WriteString("\r\n")
}

func writeMemoryInfo(b *strings.Builder, e *Engine) {
	stats := e.store.Stats()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b.WriteString("# Memory\r\n")
	fmt.Fprintf(b, "used_memory:%d\r\n", stats.MemoryBytes)
	fmt.Fprintf(b, "used_memory_human:%s\r\n", humanBytes(stats.MemoryBytes))
	fmt.Fprintf(b, "go_heap_alloc:%d\r\n", ms.HeapAlloc)
	fmt.Fprintf(b, "go_heap_alloc_human:%s\r\n", humanBytes(int64(ms.HeapAlloc)))
	fmt.Fprintf(b, "go_gc_runs:%d\r\n", ms.NumGC)
	b.WriteString("\r\n")
}

func writeStatsInfo(b *strings.Builder, e *Engine) {
	s := e.store.Stats()
	b.WriteString("# Stats\r\n")
	fmt.Fprintf(b, "keyspace_hits:%d\r\n", s.Hits)
	fmt.Fprintf(b, "keyspace_misses:%d\r\n", s.Misses)
	fmt.Fprintf(b, "expired_keys:%d\r\n", s.ExpiredKeys)
	fmt.Fprintf(b, "evicted_keys:%d\r\n", s.EvictedKeys)
	if total := s.Hits + s.Misses; total > 0 {
		fmt.Fprintf(b, "keyspace_hit_rate:%.4f\r\n", float64(s.Hits)/float64(total))
	}
	b.WriteString("\r\n")
}

func writeKeyspaceInfo(b *strings.Builder, e *Engine) {
	s := e.store.Stats()
	b.WriteString("# Keyspace\r\n")
	fmt.Fprintf(b, "db0:keys=%d,strings=%d,hashes=%d,lists=%d\r\n",
		s.Keys, s.StringKeys, s.HashKeys, s.ListKeys)
	b.WriteString("\r\n")
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
