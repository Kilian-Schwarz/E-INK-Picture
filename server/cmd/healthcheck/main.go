package main

import (
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:5000/health")
	if err != nil || resp.StatusCode != 200 {
		os.Exit(1)
	}
	resp.Body.Close()
}
