package crawler

import (
	"context"
	"os"
	"testing"

	"github.com/catouc/gitlab-ci-crawler/internal/storage"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

type NilStorage struct{}

func (ns NilStorage) CreateProjectNode(ctx context.Context, projectPath string) error {
	return nil
}

func (ns NilStorage) CreateIncludeEdge(ctx context.Context, include storage.Edge) error {
	return nil
}

func (ns NilStorage) CreateTriggerEdge(ctx context.Context, include storage.Edge) error {
	return nil
}

func (ns NilStorage) RemoveAll(ctx context.Context) error {
	return nil
}

func TestCrawlerParseTriggers(t *testing.T) {
	testFile, err := os.ReadFile("../../test-files/gitlab-ci-triggers.yaml")
	if err != nil {
		t.Fatal("cannot find test file")
	}

	crawler, err := New(&Config{}, zerolog.Logger{}, NilStorage{})
	if err != nil {
		t.Fatalf("failed to initialse crawler: %s", err)
	}

	triggers, err := crawler.parseTriggers(testFile)
	if err != nil {
		t.Fatalf("failed to parse triggers: %s", err)
	}

	expectedTriggers := []RawTrigger{
		{Project: "test/trigger"},
		{Project: "project/trigger"},
		{Project: "project/trigger", Branch: "branch"},
		{Include: "some-child/pipeline.yml"},
	}

	assert.ElementsMatch(t, expectedTriggers, triggers)
}
