package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	log.Println("Reading config file")
	
	conf := readConf("./redis.conf")
	state := NewAppState(conf)

	log.Println("Syncing AOF records")
	if conf.aofEnabled {
		state.aof.Sync()
	}

	ln, err := net.Listen("tcp", ":6379")

	if err != nil {
		log.Fatal("Cannot listen on port : 6379")
	}
	defer ln.Close()
	log.Println("Listening on 6379")
	conn, err := ln.Accept()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	for {
		v := Value{typ: "ARRAY"} //RESP Protocol always sends an array
		v.readArray(conn)

		handle(conn, &v, state)

		fmt.Println(v.array)

	}

}

type AppState struct {
	conf *Config
	aof  *Aof
}

func NewAppState(conf *Config) *AppState {
	state := AppState{
		conf: conf,
	}
	if conf.aofEnabled {
		state.aof = NewAof(conf)

		if conf.aofFsync == EverySec {
			go func() {
				t := time.NewTicker(time.Second)
				defer t.Stop()

				for range t.C {
					state.aof.w.Flush()
				}
			}()
		}
	}

	return &state
}
