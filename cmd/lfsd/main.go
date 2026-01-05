package main

import (
	"log"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/wzshiming/lfsd"
	"github.com/wzshiming/lfsd/content"
	"github.com/wzshiming/lfsd/meta"
)

func main() {

	contentStore, err := content.NewFS("lfs/content")
	if err != nil {
		log.Fatal(err)
	}

	metaStore, err := meta.NewBolt("lfs/meta.db")
	if err != nil {
		log.Fatal(err)
	}

	server := lfsd.NewServer(
		lfsd.WithContentStore(contentStore),
		lfsd.WithLocksStore(metaStore),
	)
	handler := handlers.LoggingHandler(log.Writer(), server)

	log.Println("Starting server on :8080")

	http.ListenAndServe(":8080", handler)
}
