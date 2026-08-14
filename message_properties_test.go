package notmuch

import (
	"testing"
)

func TestMessagesProperties(t *testing.T) {
	db, err := openNoConfig(dbPath, DBReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	qs := "subject:\"Introducing myself\""
	messages, err := db.NewQuery(qs).Messages()
	if err != nil {
		t.Fatalf("error getting the messages: %s", err)
	}

	var first *Message
	for first = range messages.All() {
		break
	}
	if first == nil {
		t.Fatal("couldn't get the first message")
	}

	if err := first.AddProperty("go-notmuch-test", "success"); err != nil {
		t.Fatalf("couldn't add property: %s", err)
	}

	properties := first.Properties("go-notmuch-test", true)
	for prop := range properties.All() {
		if prop.Key == "go-notmuch-test" && prop.Value == "success" {
			return
		}
	}

	t.Fatalf("couldn't find expected property")
}
