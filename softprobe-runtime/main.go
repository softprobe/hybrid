package main

import (
	"log"
	"net/http"
)

func main() {
	if err := http.ListenAndServe(defaultListenAddr, newMux()); err != nil {
		log.Fatal(err)
	}
}
