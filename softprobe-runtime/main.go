package main

import (
	"log"
	"net/http"
)

func main() {
	if err := http.ListenAndServe(listenAddr(), newMux()); err != nil {
		log.Fatal(err)
	}
}
