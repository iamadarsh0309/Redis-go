package main

import (
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	log.Println("Reading config file")
	readConf("./redis.conf")

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

		handle(conn, &v)

		fmt.Println(v.array)

	}

}
