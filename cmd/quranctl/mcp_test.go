package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMockMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "mock-session-123")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
			return
		}
		if r.Header.Get("Mcp-Session-Id") != "mock-session-123" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		params, _ := req.Params.(map[string]any)
		toolName, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)

		var structured any
		switch toolName {
		case "search_quran":
			q, _ := args["query"].(string)
			structured = map[string]any{
				"query": q,
				"results": []map[string]any{
					{
						"rank":      1,
						"ayah_key":  "2:255",
						"surah":     2,
						"ayah":      255,
						"text":      "ٱللَّهُ لَآ إِلَـٰهَ إِلَّا هُوَ",
						"relevance_score": 0.95,
						"translations": []map[string]any{
							{
								"text": "God: there is no god but Him, the Ever Living, the Ever Watchful.",
								"edition": map[string]any{
									"id":     "en-abdel-haleem",
									"name":   "The Qur'an",
									"author": "Abdel Haleem",
								},
							},
						},
					},
				},
			}
		case "fetch_quran":
			structured = map[string]any{
				"results": map[string]any{
					"ar-uthmani": []map[string]any{
						{"ayah": "2:255", "text": "ٱللَّهُ لَآ إِلَـٰهَ إِلَّا هُوَ"},
						{"ayah": "78:1", "text": "عَمَّ يَتَسَآءَلُونَ"},
					},
				},
			}
		case "fetch_translation":
			structured = map[string]any{
				"results": map[string]any{
					"en-abdel-haleem": []map[string]any{
						{"ayah": "2:255", "text": "God: there is no god but Him, the Ever Living."},
						{"ayah": "78:1", "text": "What are they asking about?"},
					},
				},
			}
		case "fetch_tafsir":
			structured = map[string]any{
				"results": map[string]any{
					"en-ibn-kathir": []map[string]any{
						{"range": "2:255", "text": "<h2>The Virtue of Ayat Al-Kursi</h2><p>This is Ayat Al-Kursi, the greatest verse in the Book of Allah.</p>"},
					},
				},
			}
		case "fetch_quran_metadata":
			if _, ok := args["juz"]; ok {
				structured = map[string]any{
					"query_type": "juz",
					"ayah":       map[string]any{"start_verse_key": "78:1", "end_verse_key": "114:6"},
					"juz":        map[string]any{"start": 30},
					"page":       map[string]any{"start": 582},
				}
			} else {
				structured = map[string]any{
					"query_type": "ayah",
					"juz":        map[string]any{"start": 3},
					"hizb":       map[string]any{"start": 5},
					"page":       map[string]any{"start": 42},
				}
			}
		}

		sb, _ := json.Marshal(structured)
		respObj := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": string(sb)},
				},
				"isError":           false,
				"structuredContent": structured,
			},
		}
		rb, _ := json.Marshal(respObj)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message\r\ndata: " + string(rb) + "\r\n\r\n"))
	}))
}

func TestCmdSearchAndAyahAndNavigate(t *testing.T) {
	srv := newMockMCPServer(t)
	defer srv.Close()

	origURL := mcpBaseURL
	mcpBaseURL = srv.URL
	defer func() { mcpBaseURL = origURL }()

	// 1. Semantic search
	var stdout, stderr bytes.Buffer
	code := cmdSearch([]string{"ayat al kursi"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdSearch failed (code %d): %s", code, stderr.String())
	}
	var searchResp SearchResponse
	if err := json.Unmarshal(stdout.Bytes(), &searchResp); err != nil {
		t.Fatalf("unmarshal search response: %v", err)
	}
	if searchResp.Mode != "semantic" || len(searchResp.Results) != 1 || searchResp.Results[0].AyahKey != "2:255" {
		t.Fatalf("unexpected search response: %+v", searchResp)
	}

	// 2. Juz search ("juz 30")
	stdout.Reset()
	stderr.Reset()
	code = cmdSearch([]string{"juz 30"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdSearch juz failed: %s", stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &searchResp); err != nil {
		t.Fatalf("unmarshal juz search: %v", err)
	}
	if searchResp.Mode != "juz" || len(searchResp.Results) != 1 || searchResp.Results[0].Surah != 78 {
		t.Fatalf("unexpected juz response: %+v", searchResp)
	}

	// 3. Ayah detail with tafsir
	stdout.Reset()
	stderr.Reset()
	code = cmdAyah([]string{"2:255", "--with-tafsir"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdAyah failed: %s", stderr.String())
	}
	var detail AyahDetailResponse
	if err := json.Unmarshal(stdout.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal ayah detail: %v", err)
	}
	if detail.AyahKey != "2:255" || !strings.Contains(detail.Tafsir, "Ayat Al-Kursi") || detail.Page != 42 {
		t.Fatalf("unexpected ayah detail: %+v", detail)
	}

	// 4. Bookmark enrichment
	bms := []LocalBookmark{{Surah: 2, Ayah: 255}}
	enrichBookmarksWithMCP(context.Background(), bms)
	if bms[0].ArabicText == "" || bms[0].Translation == "" {
		t.Fatalf("expected bookmark to be enriched, got %+v", bms[0])
	}
}
