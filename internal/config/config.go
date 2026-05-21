package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type RDBSnapshot struct {
	Secs        int
	KeysChanged int
}

type Config struct {
	Dir             string
	AOFEnabled      bool
	AOFFilename     string
	AOFFsync        string
	RDBFilename     string
	RDB             []RDBSnapshot
	MaxKeys         int
	MaxMemoryPolicy string
}

func defaults() *Config {
	return &Config{
		Dir:             ".",
		AOFFilename:     "appendonly.aof",
		AOFFsync:        "everysec",
		RDBFilename:     "dump.rdb",
		MaxMemoryPolicy: "noeviction",
	}
}

func Load(path string) *Config {
	c := defaults()
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("config: cannot read %s, using defaults\n", path)
		return c
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		parseLine(s.Text(), c)
	}
	if err := s.Err(); err != nil {
		fmt.Println("config: scan error:", err)
	}

	if c.Dir != "" {
		_ = os.MkdirAll(c.Dir, 0755)
	}
	return c
}

func parseLine(line string, c *Config) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "save":
		if len(args) < 2 {
			return
		}
		secs, err1 := strconv.Atoi(args[0])
		keys, err2 := strconv.Atoi(args[1])
		if err1 != nil || err2 != nil {
			return
		}
		c.RDB = append(c.RDB, RDBSnapshot{Secs: secs, KeysChanged: keys})
	case "dbfilename":
		c.RDBFilename = args[0]
	case "appendfilename":
		c.AOFFilename = args[0]
	case "appendfsync":
		c.AOFFsync = strings.ToLower(args[0])
	case "dir":
		c.Dir = args[0]
	case "appendonly":
		c.AOFEnabled = strings.ToLower(args[0]) == "yes"
	case "maxkeys":
		if n, err := strconv.Atoi(args[0]); err == nil {
			c.MaxKeys = n
		}
	case "maxmemory-policy":
		c.MaxMemoryPolicy = strings.ToLower(args[0])
	}
}
