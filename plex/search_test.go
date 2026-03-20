package plex

import "testing"

func TestIndexedTrackSearchStrategies_order(t *testing.T) {
	c := &Client{}
	strategies := c.indexedTrackSearchStrategies()
	if len(strategies) < 2 {
		t.Fatalf("expected multiple strategies, got %d", len(strategies))
	}
	if strategies[0].name != "exact title/artist" {
		t.Fatalf("first strategy should be exact title/artist, got %q", strategies[0].name)
	}
}
