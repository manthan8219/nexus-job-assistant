package main

import (
	"context"
	"fmt"
	"os"
	"time"
	"github.com/manthanmanthan/nexus/internal/osint"
)

func main() {
	company := "Linear"
	domain := "linear.app"
	if len(os.Args) >= 3 {
		company = os.Args[1]
		domain = os.Args[2]
	}

	finder := osint.NewFinder("", "")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Printf("Searching: %s / %s\n", company, domain)
	result := finder.Search(ctx, company, domain)
	fmt.Printf("Sources: %v\n", result.Sources)
	fmt.Printf("Errors: %v\n", result.Errors)
	fmt.Printf("Contacts: %d\n\n", len(result.Contacts))
	for _, c := range result.Contacts {
		fmt.Printf("  [%-8s %3d%%] %-25s %-35s %s\n", c.Source, c.Confidence, c.Name, c.Email, c.Notes)
	}
}
