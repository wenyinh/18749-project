package main

import (
	"flag"
	"log"
	"strings"

	"github.com/wenyinh/18749-project/rm"
)

// go run rrunner.go -addr 0.0.0.0:8001 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003"
func main() {
	addr := flag.String("addr", "0.0.0.0:8001", "RM listen address")
	serversArg := flag.String("servers", "", "")
	flag.Parse()

	serverAddrs := map[string]string{}
	if *serversArg != "" {
		pairs := strings.Split(*serversArg, ",")
		for _, p := range pairs {
			kv := strings.Split(p, "=")
			if len(kv) == 2 {
				serverAddrs[kv[0]] = kv[1]
			}
		}
	}

	r := rm.NewRM(*addr, serverAddrs)
	log.Fatal(r.Run())
}
