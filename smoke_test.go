package notmuch

// Self-contained tests for the cffi-history migration. They create their
// own database and need no fixtures, so they run anywhere - unlike the
// fixture-dependent suite, which needs the gitignored download.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func smokeWriteMsg(t *testing.T, dir, name, id string, replyTo string) string {
	t.Helper()
	headers := fmt.Sprintf("From: a@example.com\nTo: b@example.com\nSubject: smoke thread\nMessage-ID: <%s>\nDate: Thu, 01 Jan 2026 00:00:00 +0000\n", id)
	if replyTo != "" {
		headers += fmt.Sprintf("In-Reply-To: <%s>\nReferences: <%s>\n", replyTo, replyTo)
	}
	msg := headers + "\nbody\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(msg), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func smokeNewDB(t *testing.T, path string) *DB {
	t.Helper()
	db, err := CreateWithConfig(&path, &empty, nil)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

var empty = ""

func TestSmokeCreateWithConfig(t *testing.T) {
	dir := t.TempDir()
	db := smokeNewDB(t, dir)
	defer db.Close()
	if db.Version() < 1 {
		t.Errorf("version %d", db.Version())
	}

	// creating over an existing database must fail
	if _, err := CreateWithConfig(&dir, &empty, nil); err != ErrDatabaseExists {
		t.Errorf("second create: want ErrDatabaseExists got %s", err)
	}
}

func TestSmokeConfig(t *testing.T) {
	db := smokeNewDB(t, t.TempDir())
	defer db.Close()

	if _, err := db.GetConfig("missing.key"); err != ErrNotFound {
		t.Errorf("GetConfig(missing): want ErrNotFound got %s", err)
	}
	if err := db.SetConfig("test.key", "value"); err != nil {
		t.Fatal(err)
	}
	if v, err := db.GetConfig("test.key"); err != nil || v != "value" {
		t.Errorf("GetConfig: want value, got %q %s", v, err)
	}

	// defaults are present in the pairs iterator
	cfgList, err := db.GetConfigList("new.")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for key, value := range cfgList.All() {
		if key == "new.tags" {
			found = value == "unread;inbox"
		}
	}
	if !found {
		t.Error("new.tags not in config pairs")
	}

	// the key we set shows up
	cfgList, err = db.GetConfigList("test.")
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for key, value := range cfgList.All() {
		if key == "test.key" {
			found = value == "value"
		}
	}
	if !found {
		t.Error("test.key not found in config pairs")
	}
}

func TestSmokeReopen(t *testing.T) {
	db := smokeNewDB(t, t.TempDir())
	defer db.Close()
	if err := db.SetConfig("test.key", "value"); err != nil {
		t.Fatal(err)
	}
	if err := db.Reopen(DBReadOnly); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConfig("test.key2", "value"); err != ErrReadOnlyDB {
		t.Errorf("write after reopen RO: want ErrReadOnlyDB got %s", err)
	}
	if err := db.Reopen(DBReadWrite); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConfig("test.key2", "value"); err != nil {
		t.Errorf("write after reopen RW: %s", err)
	}
}

func TestSmokeAtomicAbort(t *testing.T) {
	dir := t.TempDir()
	db := smokeNewDB(t, dir)
	if err := db.SetConfig("test.key", "value"); err != nil {
		t.Fatal(err)
	}
	err := db.Atomic(func(db *DB) {
		if err := db.SetConfig("test.abort", "value"); err != nil {
			t.Errorf("SetConfig in atomic: %s", err)
		}
		db.Close()
	})
	if err != nil {
		t.Errorf("Atomic with abort: %s", err)
	}

	db2, err := OpenWithConfig(&dir, &empty, nil, DBReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	// closing a database with an open transaction discards ALL pending
	// changes, including ones made before begin_atomic
	if _, err := db2.GetConfig("test.abort"); err != ErrNotFound {
		t.Errorf("aborted change present: %s", err)
	}
	if _, err := db2.GetConfig("test.key"); err != ErrNotFound {
		t.Errorf("pre-atomic change not discarded: %s", err)
	}
}

func TestSmokeAtomicCommit(t *testing.T) {
	db := smokeNewDB(t, t.TempDir())
	defer db.Close()
	err := db.Atomic(func(db *DB) {
		if err := db.SetConfig("test.commit", "value"); err != nil {
			t.Errorf("SetConfig in atomic: %s", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := db.GetConfig("test.commit"); err != nil || v != "value" {
		t.Errorf("committed change lost: %q %s", v, err)
	}
}

func TestSmokeMatched(t *testing.T) {
	dir := t.TempDir()
	maildir := filepath.Join(dir, "mail")
	if err := os.MkdirAll(maildir, 0700); err != nil {
		t.Fatal(err)
	}
	p1 := smokeWriteMsg(t, maildir, "msg1", "smoke-1@example.com", "")
	p2 := smokeWriteMsg(t, maildir, "msg2", "smoke-2@example.com", "smoke-1@example.com")

	db := smokeNewDB(t, dir)
	defer db.Close()
	if _, err := db.AddMessage(p1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMessage(p2); err != nil {
		t.Fatal(err)
	}

	query := db.NewQuery("id:smoke-1@example.com")
	threads, err := query.Threads()
	if err != nil {
		t.Fatal(err)
	}
	var thread *Thread
	for thread = range threads.All() {
		break
	}
	if thread == nil {
		t.Fatal("no thread")
	}
	if want, got := 2, thread.Count(); want != got {
		t.Errorf("thread count: want %d got %d", want, got)
	}
	if want, got := 1, thread.CountMatched(); want != got {
		t.Errorf("matched count: want %d got %d", want, got)
	}
	matched := 0
	total := 0
	for msg := range thread.Messages().All() {
		total++
		if msg.Matched() {
			matched++
		}
	}
	if want, got := 2, total; want != got {
		t.Errorf("thread messages: want %d got %d", want, got)
	}
	if want, got := 1, matched; want != got {
		t.Errorf("matched messages: want %d got %d", want, got)
	}
}

func TestSmokeOpenConfigSearch(t *testing.T) {
	t.Setenv("NOTMUCH_CONFIG", "/nonexistent/config")
	dir := t.TempDir()
	smokeNewDB(t, dir)
	if _, err := Open(dir, DBReadOnly); err != ErrNoConfig {
		t.Errorf("Open with missing config: want ErrNoConfig got %s", err)
	}
	// an explicit empty config must not need a config file
	db, err := OpenWithConfig(&dir, &empty, nil, DBReadOnly)
	if err != nil {
		t.Errorf("OpenWithConfig empty: %s", err)
	} else {
		db.Close()
	}
}

func TestSmokeErrClosedDatabase(t *testing.T) {
	db := smokeNewDB(t, t.TempDir())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// the C library reports the closed/destroyed database
	if _, err := db.GetConfig("x"); err == nil {
		t.Error("GetConfig on closed db: expected error")
	}
}

func TestSmokeClosedDatabaseGuards(t *testing.T) {
	db := smokeNewDB(t, t.TempDir())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// every accessor must return a zero value and every operation
	// ErrClosedDatabase, never panic or crash
	if got := db.Version(); got != 0 {
		t.Errorf("Version: want 0, got %d", got)
	}
	if got := db.Path(); got != "" {
		t.Errorf("Path: want empty, got %q", got)
	}
	if db.NeedsUpgrade() {
		t.Error("NeedsUpgrade on closed db")
	}
	if _, err := db.NewQuery("").Messages(); err != ErrClosedDatabase {
		t.Errorf("query on closed db: want ErrClosedDatabase, got %s", err)
	}
	if _, err := db.AddMessage("/x"); err != ErrClosedDatabase {
		t.Errorf("AddMessage: want ErrClosedDatabase, got %s", err)
	}
}
