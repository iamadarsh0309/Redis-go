package main

import (
	"fmt"
	"net"
)

type Handler func(*Value) *Value

var Handlers = map[string]Handler{
	"COMMAND": command,
	"GET":     get,
	"SET":     set,
}

func command(v *Value) *Value {
	return &Value{typ: STRING, str: "OK"}
}

func handle(conn net.Conn, v *Value) {
	cmd := v.array[0].bulk
	handler, ok := Handlers[cmd]
	if !ok {
		fmt.Println("invalid command")
		return
	}
	reply := handler(v)
	w := NewWriter(conn)
	w.Write(reply)

}

func get(v *Value) *Value {
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

func set(v *Value) *Value {
	args := v.array[1:]
	if len(args) != 2 {
		return &Value{typ: ERROR, err: "ERR invalid number of arguments for SET command"}
	}

	key := args[0].bulk
	val := args[1].bulk
	DB.mu.Lock()
	DB.store[key] = val
	DB.mu.Unlock()
	return &Value{typ: STRING, str: "OK"}
}
