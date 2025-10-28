package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/wenyinh/18749-project/server"
)

// bin/server -role primary \
// -rid S1 -addr :9001 -init_state 0 \
// -backups "S2=10.0.0.2:9002,S3=10.0.0.3:9003" \
// -ckpt_ms 3000
func parseBackups(s string) map[string]string {
	m := make(map[string]string)
	if strings.TrimSpace(s) == "" {
		return m
	}
	pairs := strings.Split(s, ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			id := strings.TrimSpace(kv[0])
			addr := strings.TrimSpace(kv[1])
			if id != "" && addr != "" {
				m[id] = addr
			}
		}
	}
	return m
}

func main() {
	addr := flag.String("addr", ":9000", "server listen address, e.g. :9000 or 127.0.0.1:9000")
	rid := flag.String("rid", "S1", "replica id for logs")
	init := flag.Int("init_state", 0, "initial server state counter")

	roleFlag := flag.String("role", "primary", "server role: primary|backup")
	backupsFlag := flag.String("backups", "", "for primary only, comma-separated list: S2=ip:port,S3=ip:port")
	ckptMs := flag.Int("ckpt_ms", 5000, "checkpoint interval in milliseconds (primary only)")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	var role server.Role
	switch strings.ToLower(strings.TrimSpace(*roleFlag)) {
	case "primary":
		role = server.Primary
	case "backup":
		role = server.Backup
	default:
		log.Fatalf("invalid -role: %s (use primary|backup)", *roleFlag)
	}

	var backups map[string]string
	if role == server.Primary {
		backups = parseBackups(*backupsFlag)
	} else {
		backups = nil
	}
	s := server.NewServer(
		*addr,
		*rid,
		*init,
		role,
		backups,
		nil,
		time.Duration(*ckptMs)*time.Millisecond,
	)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
