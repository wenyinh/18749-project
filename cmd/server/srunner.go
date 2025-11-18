package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/wenyinh/18749-project/server"
)

// go run srunner.go -rid S1 -addr 0.0.0.0:9001 -init_state 0 -backups "S2=127.0.0.1:9002,S3=127.0.0.1:9003" -ckpt_ms 5000 -rm 127.0.0.1:8001
// go run srunner.go -rid S2 -addr 0.0.0.0:9002 -init_state 0 -backups "S1=127.0.0.1:9001,S3=127.0.0.1:9003" -ckpt_ms 5000 -rm 127.0.0.1:8001
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
	backupsFlag := flag.String("backups", "", "comma-separated list: S2=ip:port,S3=ip:port")
	ckptMs := flag.Int("ckpt_ms", 5000, "checkpoint interval in milliseconds")
	rmAddr := flag.String("rm", "", "RM address, e.g. 127.0.0.1:8001")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	backups := parseBackups(*backupsFlag)

	// 初始 role 随便给一个（例如 Backup），真正的 role 由 RM 通过 ROLE 消息指派并动态切换
	initialRole := server.Backup

	s := server.NewServer(
		*addr,
		*rid,
		*init,
		initialRole,
		backups,
		nil,
		time.Duration(*ckptMs)*time.Millisecond,
		*rmAddr,
	)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
