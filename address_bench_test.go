package notmuch

// Run: NOTMUCH_BENCH=1 go test -run TestBenchAddresses -v
// Compares the binding's chunked harvest against the CLI (the
// mutt-query baseline) on the real mailbox. Sender/To are DB-cached
// headers (zero file opens); cc/bcc open every message file - the
// fast/slow split this measures.

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestBenchAddresses(t *testing.T) {
	if os.Getenv("NOTMUCH_BENCH") == "" {
		t.Skip("set NOTMUCH_BENCH=1 to run")
	}
	dbPath := os.Getenv("NOTMUCH_DB")
	if dbPath == "" {
		dbPath = "/home/timebomb/Mail"
	}
	db, err := Open(dbPath, DBReadOnly)
	if err != nil {
		t.Fatalf("open %s: %v", dbPath, err)
	}
	defer db.Close()

	report := func(name string, fn func() (int, error)) {
		t0 := time.Now()
		n, err := fn()
		if err != nil {
			t.Logf("%-32s ERROR %v", name, err)
			return
		}
		t.Logf("%-32s %8s  (%d entries)", name, time.Since(t0).Round(time.Millisecond), n)
	}

	report("binding sender harvest", func() (int, error) {
		got, err := db.Addresses("*", AddressOpts{Sender: true})
		return len(got), err
	})
	report("binding recipients harvest", func() (int, error) {
		got, err := db.Addresses("*", AddressOpts{Recipients: true})
		return len(got), err
	})
	report("cli recipients harvest", func() (int, error) {
		out, err := exec.Command("notmuch", "address", "--deduplicate=address", "--output=recipients", "*").Output()
		if err != nil {
			return 0, err
		}
		n := 0
		for _, b := range out {
			if b == '\n' {
				n++
			}
		}
		return n, nil
	})
}
