package notmuch

import (
	"reflect"
	"testing"
)

func TestMessagesTags(t *testing.T) {
	db, err := openNoConfig(dbPath, DBReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	qs := "subject:\"Introducing myself\""
	thread, err := firstThread(db, qs)
	if err != nil {
		t.Fatal(err)
	}
	msgs := thread.Messages()
	tags := msgs.Tags().slice()
	if want, got := []string{"inbox", "signed", "unread"}, tags; !reflect.DeepEqual(want, got) {
		t.Errorf("thread.Tags(): want %v got %v", want, got)
	}
}
