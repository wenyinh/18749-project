package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/wenyinh/18749-project/server"
)

// go run srunner.go -rid S1 -addr 0.0.0.0:9001 -init_state 0 -backups "S2=127.0.0.1:9002,S3=127.0.0.1:9003" -ckpt_ms 5000
//
// recover:
// go run srunner.go -rid S2 -addr 0.0.0.0:9002 -init_state 0 -backups "S1=127.0.0.1:9001,S3=127.0.0.1:9003" -ckpt_ms 5000 -newborn
func main() {
	addr := flag.String("addr", ":9000", "server listen address, e.g. :9000 or 127.0.0.1:9000")
	rid := flag.String("rid", "S1", "replica id for logs")
	initState := flag.Int("init_state", 0, "initial server state counter")
	backupsStr := flag.String("backups", "", "backup replicas (format: ID1=addr1,ID2=addr2,...)")
	ckptMs := flag.Int("ckpt_ms", 0, "checkpoint period in ms (0 = disable)")
	newborn := flag.Bool("newborn", false, "start as newborn replica (needs checkpoint to be ready)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	backups := parseBackups(*backupsStr)
	ckptFreq := time.Duration(*ckptMs) * time.Millisecond

	s := server.NewServer(*addr, *rid, *initState, *newborn, backups, ckptFreq)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}

func parseBackups(s string) map[string]string {
	res := make(map[string]string)
	s = strings.TrimSpace(s)
	if s == "" {
		return res
	}
	pairs := strings.Split(s, ",")
	for _, p := range pairs {
		parts := strings.Split(p, "=")
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		if id != "" && addr != "" {
			res[id] = addr
		}
	}
	return res
}
