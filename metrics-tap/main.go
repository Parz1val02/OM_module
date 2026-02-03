package main

import (
	"fmt"
	"log"
	"net"
	"os"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := getenv("CHONO_IP", "0.0.0.0")
	port := getenv("CHONO_PORT", "55555")

	udpAddr, err := net.ResolveUDPAddr("udp", addr+":"+port)
	if err != nil {
		log.Fatalf("resolve addr failed: %v", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	defer func() {
		err = conn.Close()
	}()

	log.Printf("📡 srsRAN metrics listener started on %s:%s\n", addr, port)

	buf := make([]byte, 1024*1024)

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("read error: %v", err)
			continue
		}

		fmt.Println("--------------------------------------------------")
		fmt.Printf("From: %s\n", remoteAddr.String())
		fmt.Printf("Payload (%d bytes):\n", n)
		fmt.Println(string(buf[:n]))
		fmt.Println("--------------------------------------------------")
	}
}
