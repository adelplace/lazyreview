package main

import (
	"log"
	"net/http"
	"os"

	api "github.com/adelplace/lazyreview-demo/internal/http"
	"github.com/adelplace/lazyreview-demo/internal/store"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	s := store.New()
	s.Add("Write the README")
	s.Add("Review open pull requests")

	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, api.NewRouter(s)))
}
