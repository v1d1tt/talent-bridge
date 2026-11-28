// Command dev-blob prepares Azurite, the local Azure Storage emulator, for
// local runs and the Playwright tests: it creates the key-frames container
// and lets the given app origins PUT to it (CORS). It is not in the Docker
// image; on Azure the owner does the same once with the Azure CLI.
//
//	go run ./cmd/dev-blob http://localhost:8080 http://localhost:8090
package main

import (
	"cmp"
	"context"
	"log"
	"os"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/blob"
)

func main() {
	origins := os.Args[1:]
	if len(origins) == 0 {
		origins = []string{"http://localhost:8080"}
	}
	cfg := blob.Config{
		AccountURL: cmp.Or(os.Getenv("BLOB_ACCOUNT_URL"), "http://127.0.0.1:10000/devstoreaccount1"),
		Container:  cmp.Or(os.Getenv("BLOB_CONTAINER"), "key-frames"),
		AccountKey: blob.AzuriteKey,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Azurite may still be starting: try for a while.
	for {
		err := blob.SetupDev(ctx, cfg, origins)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			log.Fatalf("dev-blob: %v", err)
		}
		time.Sleep(time.Second)
	}
	log.Printf("dev-blob: container %q ready at %s, CORS for %v", cfg.Container, cfg.AccountURL, origins)
}
