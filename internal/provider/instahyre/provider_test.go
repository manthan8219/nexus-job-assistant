package instahyre

import (
	"context"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestName(t *testing.T) {
	c := New()
	if c.Name() != "instahyre" {
		t.Errorf("Name() = %q; want \"instahyre\"", c.Name())
	}
}

func TestSearch_NotImplemented(t *testing.T) {
	c := New()
	_, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error from unimplemented Search")
	}
}

func TestApply(t *testing.T) {
	c := New()
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://example.com/job/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("Apply status = %q; want \"skipped\"", res.Status)
	}
}
