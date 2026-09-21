package bugs

import (
	"context"
	"fmt"
	"testing"

	"github.com/sudarsh1010/goyon/par"
)

func TestParCaptureRace(t *testing.T) {
	ctx := t.Context()
	urls := make([]string, 20)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/%d", i)
	}

	results, err := par.Map(
		ctx,
		urls,
		func(ctx context.Context, i int, url string) (string, error) {
			return "body of " + url, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != len(urls) {
		t.Fatalf("lost updates: got %d results, want %d", len(results), len(urls))
	}
}
