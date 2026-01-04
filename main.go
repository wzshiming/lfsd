// Package main implements a Git LFS server.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var (
	version = "0.1.0"
)

func main() {
	var (
		listen      = flag.String("listen", ":8080", "address to listen on")
		host        = flag.String("host", "localhost:8080", "host used when generating URLs")
		scheme      = flag.String("scheme", "http", "URL scheme (http or https)")
		contentPath = flag.String("content-path", "lfs-content", "path to store LFS objects")
		showVersion = flag.Bool("version", false, "show version")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	config := &Config{
		Listen:      *listen,
		Host:        *host,
		Scheme:      *scheme,
		ContentPath: *contentPath,
	}

	// Override with environment variables if set
	if v := os.Getenv("LFS_LISTEN"); v != "" {
		config.Listen = v
	}
	if v := os.Getenv("LFS_HOST"); v != "" {
		config.Host = v
	}
	if v := os.Getenv("LFS_SCHEME"); v != "" {
		config.Scheme = v
	}
	if v := os.Getenv("LFS_CONTENTPATH"); v != "" {
		config.ContentPath = v
	}

	contentStore, err := NewContentStore(config.ContentPath)
	if err != nil {
		log.Fatalf("Error creating content store: %s", err)
	}

	metaStore := NewMetaStore()

	app := NewApp(config, contentStore, metaStore)

	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		log.Fatalf("Error creating listener: %s", err)
	}

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down...")
		listener.Close()
	}()

	log.Printf("Git LFS server listening on %s", config.Listen)
	if err := http.Serve(listener, app); err != nil && err != http.ErrServerClosed {
		log.Printf("Server error: %s", err)
	}
}
