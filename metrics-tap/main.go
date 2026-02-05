package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	SRS_GNB_IP := os.Getenv("SRS_GNB_IP")
	if SRS_GNB_IP == "" {
		log.Fatal("SRS_GNB_IP not set")
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+SRS_GNB_IP+":8001", nil)
	if err != nil {
		log.Fatal("dial error:", err)
	}
	defer func() {
		err = conn.Close()
	}()

	sub := map[string]string{"cmd": "metrics_subscribe"}
	if err := conn.WriteJSON(sub); err != nil {
		log.Fatal("subscribe error:", err)
	}

	log.Println("✅ Subscribed to gNB metrics")

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("connection closed:", err)
			time.Sleep(time.Second)
			return
		}

		var metric map[string]any
		if err := json.Unmarshal(msg, &metric); err != nil {
			continue
		}

		if _, ok := metric["cmd"]; ok {
			continue
		}

		pretty, _ := json.MarshalIndent(metric, "", "  ")
		log.Println(string(pretty))
	}
}
