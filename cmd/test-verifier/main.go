// Package main runs the SMTP email verifier against addresses passed on the
// command line and prints the outcome for each one. It is a thin CLI over
// internal/osint.Verifier — no business logic lives here.
//
// Usage:
//
//	go run ./cmd/test-verifier alice@acme.com bob@example.com ...
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: test-verifier <email1> [email2 ...]\n")
		os.Exit(2)
	}
	ver := osint.NewVerifier()
	for _, email := range os.Args[1:] {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		v := ver.Verify(ctx, email)
		cancel()
		fmt.Printf("%-48s %-12s %3d%%  %-30s %s\n", email, v.Status, v.Confidence, v.Reason, v.Detail)
	}
}
