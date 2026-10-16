package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	// Single reading bookmark object returned by /api/proxy/auth/bookmarks?mushafId=2&limit=1&isReading=true
	rawSingle := []byte(`{"id":"rb-single","key":18,"type":"ayah","verseNumber":10}`)
	singleList, err := parseBookmarksPayload(rawSingle)
	if err != nil {
		t.Fatalf("parseBookmarksPayload single object failed: %v", err)
	}
	if len(singleList) != 1 || singleList[0].Key != 18 || singleList[0].VerseNumber != 10 {
		t.Fatalf("unexpected single object item: %+v", singleList)
	}

	// Page-type reading bookmark (page 293 -> Surah 18 Al-Kahf)
	rawPage := []byte(`{"id":"rb-page","key":293,"type":"page"}`)
	pageList, err := parseBookmarksPayload(rawPage)
	if err != nil {
		t.Fatalf("parseBookmarksPayload page object failed: %v", err)
	}
	if len(pageList) != 1 || pageList[0].Key != 18 || pageList[0].VerseNumber != 1 {
		t.Fatalf("unexpected page bookmark conversion: %+v", pageList)
	}

	// Pinned items payload
	rawPinned := []byte(`{"success":true,"data":{"data":[{"id":"pin1","targetType":"ayah","targetId":"2:255","createdAt":"2026-10-15T00:00:00Z"}]}}`)
	pinnedList, err := parseBookmarksPayload(rawPinned)
	if err != nil {
		t.Fatalf("parseBookmarksPayload pinned items failed: %v", err)
	}
	if len(pinnedList) != 1 || pinnedList[0].Key != 2 || pinnedList[0].VerseNumber != 255 {
		t.Fatalf("unexpected pinned item: %+v", pinnedList)
	}

	// Reading sessions payload
	rawSessions := []byte(`[{"id":"s1","chapterNumber":67,"verseNumber":3,"updatedAt":"2026-10-15T00:00:00Z"}]`)
	sessionList, err := parseBookmarksPayload(rawSessions)
	if err != nil {
		t.Fatalf("parseBookmarksPayload sessions failed: %v", err)
	}
	if len(sessionList) != 1 || sessionList[0].Key != 67 || sessionList[0].VerseNumber != 3 {
		t.Fatalf("unexpected reading session item: %+v", sessionList)
	}
}

func TestAuthRouteAndJWTExtraction(t *testing.T) {
	// Token with payload {"refreshToken":"test-refresh-tok-123"}
	jwtTok := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWZyZXNoVG9rZW4iOiJ0ZXN0LXJlZnJlc2gtdG9rLTEyMyJ9.sig"
	if got := extractRefreshTokenFromJWT(jwtTok); got != "test-refresh-tok-123" {
		t.Fatalf("expected refreshToken test-refresh-tok-123, got %q", got)
	}

	// Stub HTTP client so exchangeQuranComRedirectToken succeeds offline
	origFactory := newSyncHTTPClient
	defer func() { newSyncHTTPClient = origFactory }()
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "at", Value: "mock-access-tok", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "rt", Value: "mock-refresh-tok", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	defer mockBackend.Close()

	newSyncHTTPClient = func() *http.Client {
		return &http.Client{
			Timeout: 5 * time.Second,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				u := mockBackend.URL + req.URL.RequestURI()
				proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method, u, nil)
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(proxyReq)
			}),
		}
	}

	resultCh := make(chan AuthConfig, 1)
	errCh := make(chan error, 1)
	mux := buildLoginMux(AuthConfig{Mode: "web"}, "google", "http://127.0.0.1:19450/callback", "st", "nc", "ver", "chal", resultCh, errCh)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:19450/auth?token="+jwtTok, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /auth?token=..., got %d (%s)", rec.Code, rec.Body.String())
	}
	select {
	case cfg := <-resultCh:
		if cfg.Cookies["at"] != "mock-access-tok" || cfg.Cookies["rt"] != "mock-refresh-tok" {
			t.Fatalf("unexpected cookies from /auth callback: %+v", cfg.Cookies)
		}
	default:
		t.Fatal("expected AuthConfig on resultCh after /auth?token=... request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

