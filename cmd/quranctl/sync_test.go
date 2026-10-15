package main

import (
	"testing"
)

func TestParseAndDeduplicateBookmarks(t *testing.T) {
	rawArray := []byte(`[
		{"id":"bm1","createdAt":"2026-10-14T23:19:48.764Z","type":"ayah","key":42,"verseNumber":5,"isReading":true},
		{"id":"bm2","createdAt":"2026-10-14T17:22:15.553Z","type":"ayah","key":41,"verseNumber":1,"isReading":null},
		{"id":"bm3","createdAt":"2026-10-14T17:22:15.553Z","type":"ayah","key":42,"verseNumber":5}
	]`)
	list, err := parseBookmarksPayload(rawArray)
	if err != nil {
		t.Fatalf("parseBookmarksPayload array failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 raw items, got %d", len(list))
	}
	dedup := deduplicateRemoteBookmarks(list)
	if len(dedup) != 2 {
		t.Fatalf("expected 2 deduplicated items, got %d", len(dedup))
	}
	if dedup[0].Key != 42 || dedup[0].VerseNumber != 5 || dedup[0].IsReading == nil || !*dedup[0].IsReading {
		t.Fatalf("unexpected first item: %+v", dedup[0])
	}
	if dedup[1].Key != 41 || dedup[1].VerseNumber != 1 {
		t.Fatalf("unexpected second item: %+v", dedup[1])
	}

	rawEnvelope := []byte(`{"success":true,"data":[{"id":"r1","type":"ayah","key":18,"verseNumber":10}]}`)
	envList, err := parseBookmarksPayload(rawEnvelope)
	if err != nil {
		t.Fatalf("parseBookmarksPayload envelope failed: %v", err)
	}
	if len(envList) != 1 || envList[0].Key != 18 || envList[0].VerseNumber != 10 {
		t.Fatalf("unexpected envelope item: %+v", envList)
	}
}
