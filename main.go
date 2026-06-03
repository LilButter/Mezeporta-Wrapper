package main

import (
	"log"
	"os"
)

func main() {
	root, err := currentServerRoot()
	if err != nil {
		log.Fatalf("failed to determine server root: %v", err)
	}

	app, err := newApp(root)
	if err != nil {
		log.Fatalf("failed to initialize wrapper: %v", err)
	}

	if err := app.Start(); err != nil {
		_ = app.Shutdown()
		log.Fatalf("failed to start wrapper: %v", err)
	}

	if err := app.Wait(); err != nil {
		log.Printf("wrapper stopped with error: %v", err)
		os.Exit(1)
	}
}
