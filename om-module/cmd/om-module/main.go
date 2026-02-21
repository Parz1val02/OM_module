package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type Response struct {
	Messsage string `json:"message"`
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	response := Response{
		Messsage: "Hello from the OM module",
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error encoding response: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func main() {
	http.HandleFunc("/hello", helloHandler)
	log.Println("Server running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
