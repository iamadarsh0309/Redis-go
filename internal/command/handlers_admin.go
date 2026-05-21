package command

import (
	"log"

	"go-redis/internal/persistence"
	"go-redis/internal/protocol"
)

func cmdDBSize(e *Engine, v *protocol.Value) *protocol.Value {
	return intReply(int64(e.store.Len()))
}

func cmdFlushDB(e *Engine, v *protocol.Value) *protocol.Value {
	e.store.FlushAll()
	logCommand(e, v)
	return okReply()
}

func cmdSave(e *Engine, v *protocol.Value) *protocol.Value {
	if e.rdbPath == "" {
		return errReply("ERR RDB is not configured (set 'dir' and 'dbfilename')")
	}
	snap := Snapshot(e.store)
	if err := persistence.WriteRDB(e.rdbPath, snap); err != nil {
		log.Println("rdb save:", err)
		return errReply("ERR " + err.Error())
	}
	e.markSaved()
	return okReply()
}

func cmdBGSave(e *Engine, v *protocol.Value) *protocol.Value {
	if e.rdbPath == "" {
		return errReply("ERR RDB is not configured")
	}
	e.lastSaveMu.Lock()
	if e.bgSaving {
		e.lastSaveMu.Unlock()
		return errReply("ERR Background save already in progress")
	}
	e.bgSaving = true
	e.lastSaveMu.Unlock()

	snap := Snapshot(e.store)
	go func() {
		defer func() {
			e.lastSaveMu.Lock()
			e.bgSaving = false
			e.lastSaveMu.Unlock()
		}()
		if err := persistence.WriteRDB(e.rdbPath, snap); err != nil {
			log.Println("bgsave:", err)
			return
		}
		e.markSaved()
		log.Println("bgsave complete:", e.rdbPath)
	}()
	return &protocol.Value{Type: protocol.String, Str: "Background saving started"}
}

func cmdBGRewriteAOF(e *Engine, v *protocol.Value) *protocol.Value {
	if e.aof == nil {
		return errReply("ERR AOF is disabled")
	}
	c, ok := e.aof.(persistence.Compactor)
	if !ok {
		return errReply("ERR AOF backend does not support compaction")
	}
	e.lastSaveMu.Lock()
	if e.bgRewriting {
		e.lastSaveMu.Unlock()
		return errReply("ERR Background AOF rewrite already in progress")
	}
	e.bgRewriting = true
	e.lastSaveMu.Unlock()

	go func() {
		defer func() {
			e.lastSaveMu.Lock()
			e.bgRewriting = false
			e.lastSaveMu.Unlock()
		}()
		if err := c.Compact(func() []*protocol.Value { return Snapshot(e.store) }); err != nil {
			log.Println("bgrewriteaof:", err)
			return
		}
		log.Println("bgrewriteaof complete")
	}()
	return &protocol.Value{Type: protocol.String, Str: "Background append only file rewriting started"}
}

func cmdLastSave(e *Engine, v *protocol.Value) *protocol.Value {
	return intReply(e.LastSave().Unix())
}

func (e *Engine) markSaved() {
	e.lastSaveMu.Lock()
	defer e.lastSaveMu.Unlock()
	e.lastSaveAt = timeNow()
}
