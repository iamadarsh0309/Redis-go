package main

import (
	"bufio"
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
	ERROR ValueType = "-"
	NULL ValueType = ""
)

type Value struct {
	typ ValueType
	bulk string
	str string
	err string
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
	v:= Value{typ: "ARRAY"} //RESP Protocol always sends an array
	v.readArray(conn)


	fmt.Println(v.array)


	conn.Write([]byte("+OK\r\n"))

}

}

type Handler func(*Value) *Value

var Handlers = map[string]Handler{}

func handle(conn net.Conn, v *Value){
	cmd := v.array[0].bulk
	handler, ok := Handlers[cmd]
	if !ok {
		fmt.Println("invalid command")
		return
	}
	reply :=  handler(v)
	w:= NewWriter(conn)
	w.Write(reply)

}

type Writer struct {
	writer io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{writer: bufio.NewWriter(w)}
}

func (w *Writer) Write(v *Value) {
	var reply string
	switch v.typ {
	case STRING:
		reply= fmt.Sprintf("%s%s\r\n", v.typ, v.str)
	case BULK:
		reply = fmt.Sprintf("%s%d\r\n%s\r\n", v.typ, len(v.bulk), v.bulk)
	case ERROR:
		reply=fmt.Sprintf("%s%s\r\n", v.typ, v.err)
	case NULL:
		reply=fmt.Sprintf("$-1\r\n")
	}

	w.writer.Write([]byte(reply))
}