package companies

import (
	"fmt"
	"os"
	"sync"
)

// networkSeedOnce ensures only one background network-seed goroutine runs
// per process, no matter how many times OpenDefault() is called (the UI,
// engine, and careerscraper all open their own *DB handles independently).
var networkSeedOnce sync.Once

// ensureSeeded populates a freshly created database from Nexus' embedded
// company data (ATS board lists + India priority employers) so the app is
// usable immediately after install, without requiring a manual
// `companies-seed` run. It only acts when the database is empty — an
// already-seeded database is left untouched.
//
// It then checks whether the network-backed sources (OpenJobs, Y
// Combinator) have ever been imported; if not, it kicks off a background
// fetch so first launch isn't blocked on network calls (the YC import
// alone is ~200+ HTTP requests). Once either source has any rows, it's
// considered seeded and won't be re-fetched automatically — use
// RefreshCompanies (wired to the Companies tab's refresh action) to force
// a re-pull.
// seedEmbeddedOnly imports the embedded catalogs (boards + India employers)
// when the DB is empty. Never touches the network.
func (s *DB) seedEmbeddedOnly() {
	n, err := s.Count()
	if err == nil && n == 0 {
		if _, err := s.ImportNexusEmbeddedBoards(); err != nil {
			fmt.Fprintf(os.Stderr, "companies: auto-seed boards failed: %v\n", err)
		}
		if _, err := s.ImportIndiaEmployers(); err != nil {
			fmt.Fprintf(os.Stderr, "companies: auto-seed india employers failed: %v\n", err)
		}
	}
}

func (s *DB) ensureSeeded(dbPath string) {
	s.seedEmbeddedOnly()

	ycCount, _ := s.CountBySource("ycombinator")
	ojCount, _ := s.CountBySource("openjobs")
	needYC := ycCount == 0
	needOpenJobs := ojCount == 0
	if !needYC && !needOpenJobs {
		return
	}
	networkSeedOnce.Do(func() {
		go runNetworkSeed(dbPath, needYC, needOpenJobs)
	})
}

// runNetworkSeed fetches the network-backed sources in the background.
// It opens its own DB handle since the caller's *DB may be closed
// independently of this goroutine's lifetime.
func runNetworkSeed(dbPath string, needYC, needOpenJobs bool) {
	db, err := Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "companies: background seed: open db: %v\n", err)
		return
	}
	defer db.Close()

	if needOpenJobs {
		n, err := db.RefreshFromOpenJobs("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "companies: background seed: openjobs: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "companies: background seed: openjobs upserted %d rows\n", n)
		}
	}
	if needYC {
		n, err := db.RefreshFromYCombinator("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "companies: background seed: ycombinator: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "companies: background seed: ycombinator upserted %d rows\n", n)
		}
	}
}

// RefreshCompanies re-pulls all network-backed sources unconditionally
// (unlike ensureSeeded, which only fetches once). Intended for a manual
// "refresh companies" action, e.g. from the Companies tab UI. Runs
// synchronously — callers that want it non-blocking should invoke it in
// their own goroutine and report progress via their own channel.
func RefreshCompanies() (int, error) {
	path, err := defaultDBPath()
	if err != nil {
		return 0, err
	}
	db, err := Open(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	total := 0
	n, err := db.RefreshFromOpenJobs("")
	if err != nil {
		return total, fmt.Errorf("openjobs: %w", err)
	}
	total += n

	n, err = db.RefreshFromYCombinator("")
	if err != nil {
		return total, fmt.Errorf("ycombinator: %w", err)
	}
	total += n

	n, err = db.ImportNexusEmbeddedBoards()
	if err != nil {
		return total, fmt.Errorf("boards: %w", err)
	}
	total += n

	n, err = db.ImportIndiaEmployers()
	if err != nil {
		return total, fmt.Errorf("india employers: %w", err)
	}
	total += n

	return total, nil
}
