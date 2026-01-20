package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
)

type Aof struct {
	w    *Writer
	f    *os.File
	conf *Config
}

func NewAof(conf *Config) *Aof {
	aof := Aof{
		conf: conf,
	}

	filePath := path.Join(aof.conf.dir, aof.conf.aoFfn)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("Cannot open this file path")
		return &aof
	}
	aof.w = NewWriter(f)
	aof.f = f
	return &aof
}

func (aof *Aof) Sync() {
	for {
		v := Value{}
		err := v.readArray(aof.f)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println("Unexpected error while reading AOF records: ", err)
			break
		}

		blankState := NewAppState(&Config{})
		set(&v, blankState)

	}
}
