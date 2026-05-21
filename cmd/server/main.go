package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"go-redis/internal/command"
	"go-redis/internal/config"
	"go-redis/internal/persistence"
	"go-redis/internal/protocol"
	"go-redis/internal/store"
)

const (
	listenAddr      = ":6379"
	maxClients      = 1024
	idleTimeout     = 5 * time.Minute
	writeTimeout    = 10 * time.Second
	shutdownGrace   = 5 * time.Second
	expirerInterval = 100 * time.Millisecond
	expirerSample   = 100
)

func main() {
	log.Println("loading config")
	cfg := config.Load("./redis.conf")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	s := store.NewMap()
	if cfg.MaxKeys > 0 {
		s.SetCapacity(cfg.MaxKeys, store.EvictionPolicy(cfg.MaxMemoryPolicy))
		log.Printf("eviction: maxkeys=%d policy=%s", cfg.MaxKeys, cfg.MaxMemoryPolicy)
	}

	var (
		aofImpl *persistence.AOF
		logger  persistence.Logger
	)
	if cfg.AOFEnabled {
		path := filepath.Join(cfg.Dir, cfg.AOFFilename)
		a, err := persistence.New(path, persistence.FSyncMode(cfg.AOFFsync))
		if err != nil {
			log.Fatalf("aof open %s: %v", path, err)
		}
		aofImpl = a
		logger = a
	}

	engine := command.NewEngine(s, logger)
	rdbPath := filepath.Join(cfg.Dir, cfg.RDBFilename)
	engine.SetRDBPath(rdbPath)

	if aofImpl != nil {
		log.Println("replaying AOF")
		if err := aofImpl.Replay(engine.Apply); err != nil {
			log.Println("replay error:", err)
		}
		if persistence.FSyncMode(cfg.AOFFsync) == persistence.EverySec {
			aofImpl.StartFlushLoop(ctx)
		}
	} else {
		log.Printf("loading RDB from %s", rdbPath)
		if err := persistence.LoadRDB(rdbPath, engine.Apply); err != nil {
			log.Println("rdb load:", err)
		}
	}

	startActiveExpirer(ctx, s)
	startPeriodicSave(ctx, engine, s, cfg)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}
	log.Printf("listening on %s", listenAddr)

	go func() {
		<-ctx.Done()
		log.Println("shutdown signal received, closing listener")
		ln.Close()
	}()

	sem := make(chan struct{}, maxClients)
	var wg sync.WaitGroup

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			log.Println("accept:", err)
			continue
		}

		select {
		case sem <- struct{}{}:
		default:
			log.Println("max clients reached, rejecting")
			conn.Close()
			continue
		}

		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			defer func() { <-sem }()
			defer c.Close()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("conn %s panic: %v\n%s",
						c.RemoteAddr(), r, debug.Stack())
				}
			}()
			handleConn(ctx, c, engine)
		}(conn)
	}

	log.Println("draining in-flight connections")
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		log.Println("shutdown grace period exceeded, forcing exit")
	}

	if aofImpl != nil {
		log.Println("flushing and closing AOF")
		if err := aofImpl.Close(); err != nil {
			log.Println("aof close:", err)
		}
	}
	log.Println("shutdown complete")
}

func startPeriodicSave(ctx context.Context, e *command.Engine, s store.Store, cfg *config.Config) {
	if len(cfg.RDB) == 0 {
		return
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		lastChanges := s.Stats().Changes
		lastTime := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				stats := s.Stats()
				deltaChanges := stats.Changes - lastChanges
				elapsed := int(time.Since(lastTime).Seconds())
				for _, rule := range cfg.RDB {
					if elapsed >= rule.Secs && deltaChanges >= int64(rule.KeysChanged) {
						log.Printf("save rule matched: %ds, %d changes — triggering BGSAVE",
							rule.Secs, deltaChanges)
						bgsave := &protocol.Value{
							Type:  protocol.Array,
							Array: []protocol.Value{{Type: protocol.Bulk, Bulk: "BGSAVE"}},
						}
						e.Execute(bgsave)
						lastChanges = stats.Changes
						lastTime = time.Now()
						break
					}
				}
			}
		}
	}()
}

func startActiveExpirer(ctx context.Context, s store.Store) {
	go func() {
		t := time.NewTicker(expirerInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.Sweep(time.Now().UnixMilli(), expirerSample)
			}
		}
	}()
}

func handleConn(ctx context.Context, c net.Conn, engine *command.Engine) {
	parser := protocol.NewParser(c)
	writer := protocol.NewWriter(c)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = c.SetReadDeadline(time.Now().Add(idleTimeout))
		v, err := parser.ReadValue()
		if err != nil {
			return
		}

		reply := engine.Execute(v)

		_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := writer.Write(reply); err != nil {
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}
