// Command supabase-check verifies the configured Supabase project reachability:
// the Postgres database (ping) and the object storage (bucket list + resume
// bucket presence). Run it any time to confirm the storage wiring is correct.
//
// Usage:
//
//	supabase-check                          check config-driven Supabase
//	supabase-check -url ... -db ... -key ...  check explicit values
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/supabase"
)

func main() {
	url := flag.String("url", "", "Supabase project URL (overrides config)")
	db := flag.String("db", "", "Postgres connection string (overrides config)")
	key := flag.String("key", "", "service_role key (overrides config; never logged)")
	flag.Parse()

	var cl *supabase.Client
	switch {
	case *url != "":
		cl = supabase.New(*url, *key, *db)
	default:
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config: ", err)
			os.Exit(1)
		}
		cl = supabase.FromConfig(cfg)
	}
	if cl == nil {
		fmt.Fprintln(os.Stderr, "Supabase is not configured. Set supabase_url (+ keys) in config, or pass -url/-db/-key.")
		os.Exit(1)
	}

	res := cl.Check(context.Background())
	fmt.Println("Supabase check:")
	fmt.Println(res.String())
	if !res.OK() {
		fmt.Println("\nNOT OK - fix the failures above (check the DB URL password, service key, and the 'resumes' bucket).")
		os.Exit(1)
	}
	fmt.Println("\nOK - database + storage reachable, resumes bucket present.")
}
