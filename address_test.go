package notmuch

// Self-contained address-harvest tests: parser unit cases plus a DB-backed
// harvest in the smoke style (own database, no fixtures).

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseMailboxes(t *testing.T) {
	cases := []struct {
		in  string
		out [][2]string
	}{
		{`Alice <alice@example.com>, Bob <bob@example.com>`,
			[][2]string{{"Alice", "alice@example.com"}, {"Bob", "bob@example.com"}}},
		{"alice@example.com",
			[][2]string{{"", "alice@example.com"}}},
		{`"Doe, John" <john.doe@example.com>`,
			[][2]string{{"Doe, John", "john.doe@example.com"}}},
		{"Team: a@x.com, b@y.com; c@z.com",
			[][2]string{{"", "a@x.com"}, {"", "b@y.com"}, {"", "c@z.com"}}},
		{"Alice (chief) <a@x.com>",
			[][2]string{{"Alice", "a@x.com"}}},
		{"=?utf-8?B?aGVsbG8=?= <x@y>",
			[][2]string{{"hello", "x@y"}}},
		{"=?utf-8?Q?hello_world?= <x@y>",
			[][2]string{{"hello world", "x@y"}}},
		{"not-an-address", nil},
		{"Undisclosed recipients:;", nil},
		{"", nil},
		{"bluecher@aks-steuerberatung.de>", nil},                 // stray trailing >
		{"Name <a@b.com> garbage", [][2]string{{"Name", "a@b.com"}}}, // junk after the close
		{"<dan>", nil},                                           // angle addr without @
		{"a@b.com\r\nsubject: x", nil},                           // header run into body
		{`'x"a@b.com""'junk'"c@d.com"`, nil},                     // unbalanced quote-run
		{"a b@c", nil},                                           // space inside the addr
	}
	for _, c := range cases {
		if got := parseMailboxes(c.in); !reflect.DeepEqual(got, c.out) {
			t.Errorf("%q: want %v, got %v", c.in, c.out, got)
		}
	}
}

// addressMsg writes a mail file with the given from/to/cc headers and
// indexes it.
func addressMsg(t *testing.T, db *DB, dir, name string, from, to, cc string) {
	t.Helper()
	headers := "Message-ID: <" + name + "@x>\nDate: Thu, 01 Jan 2026 00:00:00 +0000\n"
	if from != "" {
		headers += "From: " + from + "\n"
	}
	if to != "" {
		headers += "To: " + to + "\n"
	}
	if cc != "" {
		headers += "Cc: " + cc + "\n"
	}
	headers += "Subject: s\n\nbody\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(headers), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMessage(p); err != nil {
		t.Fatal(err)
	}
}

func TestAddressesSender(t *testing.T) {
	dir := t.TempDir()
	db := smokeNewDB(t, dir)
	defer db.Close()

	// alice appears 3 times with 3 name variants: most-popular wins with
	// a conflated count, ties break lexicographic (non-empty names first).
	addressMsg(t, db, dir, "m0", "Alice <alice@example.com>", "Bob <bob@example.com>", "dave@example.com")
	addressMsg(t, db, dir, "m1", `"Alice Smith" <alice@example.com>`, "carol@example.com", "")
	addressMsg(t, db, dir, "m2", "alice@example.com", "bob@example.com", "")

	got, err := db.Addresses("*", AddressOpts{})
	if err != nil {
		t.Fatal(err)
	}
	want := []AddressEntry{
		{Addr: "alice@example.com", Name: "Alice", Count: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sender harvest: want %v, got %v", want, got)
	}

	// recipients: to/cc only, no from
	got, err = db.Addresses("*", AddressOpts{Recipients: true})
	if err != nil {
		t.Fatal(err)
	}
	want = []AddressEntry{
		{Addr: "bob@example.com", Name: "Bob", Count: 2},
		{Addr: "dave@example.com", Name: "", Count: 1},
		{Addr: "carol@example.com", Name: "", Count: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recipient harvest: want %v, got %v", want, got)
	}

	// both: from + to/cc/bcc in one walk, first-seen order
	got, err = db.Addresses("*", AddressOpts{Sender: true, Recipients: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("combined harvest: want 4 entries, got %v", got)
	}

	// dedup key is case-insensitive
	addressMsg(t, db, dir, "m3", "ALICE@EXAMPLE.COM", "", "")
	got, err = db.Addresses("*", AddressOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Addr != "alice@example.com" || got[0].Count != 4 {
		t.Fatalf("case-insensitive dedup: want 1 entry count 4, got %v", got)
	}

	// limit stops the walk mid-harvest
	got, err = db.Addresses("*", AddressOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Addr != "alice@example.com" {
		t.Fatalf("limit: want alice only, got %v", got)
	}

	// empty result
	got, err = db.Addresses("tag:nonexistent", AddressOpts{})
	if err != nil || len(got) != 0 {
		t.Fatalf("no matches: want 0 entries nil err, got %v %v", got, err)
	}

	// harvest never panics on a truncated arena - every read is
	// bounds-checked and errors with ErrMalformedData
	truncated := [][]byte{
		{},                                        // missing count header
		{1, 0, 0, 0},                              // count says 1, no message
		{1, 0, 0, 0, 1, 0, 0, 0, 9, 0, 0, 0, 'a'}, // header claims 9 bytes, has 1
	}
	for _, data := range truncated {
		buckets := make(map[string]*addrBucket)
		var order []string
		if err := harvest(data, buckets, &order); err != ErrMalformedData {
			t.Errorf("truncated %v: want ErrMalformedData, got %v", data, err)
		}
	}
	ok := []byte{1, 0, 0, 0, 1, 0, 0, 0, 3, 0, 0, 0, 'a', '@', 'b'}
	buckets := make(map[string]*addrBucket)
	var order []string
	if err := harvest(ok, buckets, &order); err != nil {
		t.Fatalf("valid arena: %v", err)
	}
	if len(order) != 1 || buckets[order[0]].addr != "a@b" {
		t.Fatalf("valid arena: want [a@b], got %v", order)
	}

	// group + encoded-word names survive the full path
	addressMsg(t, db, dir, "m4", "", "=?utf-8?Q?Team_List?=: zoe@x.com;", "")
	got, err = db.Addresses("*", AddressOpts{Recipients: true})
	if err != nil {
		t.Fatal(err)
	}
	// group members are bare addresses (the group display name is not
	// attached, GMime semantics) - harvest must find them and not choke
	// on the encoded-word group name
	found := false
	for _, e := range got {
		if e.Addr == "zoe@x.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("group member missing, got %v", got)
	}
}
