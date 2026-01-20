package main

import (
	"fmt"
	"log"
	"net"
)

type Handler func(v *Value, state *AppState) *Value

var Handlers = map[string]Handler{
	"COMMAND": command,
	"GET":     get,
	"SET":     set,
}

func command(v *Value, state *AppState) *Value {
	return &Value{typ: STRING, str: "OK"}
}

func handle(conn net.Conn, v *Value, state *AppState) {
	cmd := v.array[0].bulk
	handler, ok := Handlers[cmd]
	if !ok {
		fmt.Println("invalid command")
		return
	}
	reply := handler(v, state)
	w := NewWriter(conn)
	w.Write(reply)
	w.Flush()

}

func get(v *Value, state *AppState) *Value {
	args := v.array[1:]

	if len(args) != 1 {
		return &Value{typ: ERROR, err: "ERR invalid number of arguments for GET Command"}
	}

	name := args[0].bulk
	DB.mu.RLock()
	val, ok := DB.store[name]
	DB.mu.RUnlock()
	if !ok {
		return &Value{typ: NULL}
	}

	return &Value{typ: BULK, bulk: val}

}

func set(v *Value, state *AppState) *Value {
	args := v.array[1:]
	if len(args) != 2 {
		return &Value{typ: ERROR, err: "ERR invalid number of arguments for SET command"}
	}

	key := args[0].bulk
	val := args[1].bulk
	DB.mu.Lock()

	DB.store[key] = val
	if state.conf.aofEnabled {
		log.Println("saving AOF record")
		state.aof.w.Write(v)

		if state.conf.aofFsync == Always {
			state.aof.w.Flush()
		}
	}


	DB.mu.Unlock()
	return &Value{typ: STRING, str: "OK"}
}
