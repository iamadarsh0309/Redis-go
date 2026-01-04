package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

type ValueType string

const (
	ARRAY ValueType = "*"
	BULK ValueType = "$"
	STRING ValueType = "+"
)

type Value struct {
	typ ValueType
	bulk string
	str string
	array []Value
}

func (v *Value) readArray(reader io.Reader){
	buf:= make([]byte,4)
	reader.Read(buf)
	arrLen, err := strconv.Atoi(string(buf[1]))
	if err!=nil {
		fmt.Println(err)
		return
	}

	for range arrLen {
		bulk := v.readBulk(reader)
		v.array = append(v.array, bulk)
	}

}

func (v *Value) readBulk (reader io.Reader) Value {
	buf := make([]byte, 4)
	reader.Read(buf)

	n,err := strconv.Atoi(string(buf[1]))
	if err!=nil {
		fmt.Println(err)
		return Value{}

	}

	bulkBuf := make([]byte, n+2)
	reader.Read(bulkBuf)

	bulk := string(bulkBuf[:n])
	return Value{typ: BULK, bulk:bulk}

}


func main() {
	ln, err := net.Listen("tcp", ":6379")

	if err!=nil {
		log.Fatal("Cannot listen on port : 6379")
	}
	defer ln.Close()

	conn, err := ln.Accept()
	if err!=nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	for {
	v:= Value{typ: "ARRAY"}
	v.readArray(conn)


	fmt.Println(v.array)
	

	conn.Write([]byte("+OK\r\n"))

}

}
