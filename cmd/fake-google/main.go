// Command fake-google runs a fake Google sign-in on this machine, for local
// development and the Playwright tests. It is not in the Docker image, and
// the server trusts it only when both run on localhost (auth.LoadConfig).
//
//	go run ./cmd/fake-google            # serves http://localhost:9999
//
// Then start the server with:
//
//	GOOGLE_ISSUER=http://localhost:9999 GOOGLE_CLIENT_ID=fake-client GOOGLE_CLIENT_SECRET=fake-secret
package main

import (
	"cmp"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/auth/fakegoogle"
)

func main() {
	port := cmp.Or(os.Getenv("FAKE_GOOGLE_PORT"), "9999")
	issuer := "http://localhost:" + port
	p, err := fakegoogle.New(issuer, "fake-client", "fake-secret")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("fake Google sign-in on %s (client id fake-client, secret fake-secret)", issuer)
	srv := &http.Server{Addr: "localhost:" + port, Handler: p.Handler(), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
