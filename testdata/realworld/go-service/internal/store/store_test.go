package store

import (
	"context"
	"os"
	"testing"
)

func TestPostgresRoundTrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := s.AddEntry(ctx, Entry{Account: "acct-1", AmountCents: 1250, Memo: "test"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListEntries(ctx, "acct-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || list[0].ID != got.ID {
		t.Fatalf("entry %d not listed: %+v", got.ID, list)
	}
}
