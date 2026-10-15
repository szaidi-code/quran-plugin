// mcp.go — Integration with Quran Foundation's MCP server (https://mcp.quran.ai)
// for semantic ayah search, live ayah Arabic + translation retrieval, tafsir
// lookup, Juz/Page/Hizb navigation, and bookmark verse enrichment.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var mcpBaseURL = "https://mcp.quran.ai"

const (
	defaultTranslationEdition = "en-abdel-haleem"
	defaultArabicEdition      = "ar-uthmani"
	defaultTafsirEdition      = "en-ibn-kathir"
	mcpGroundingNonce         = "quranctl-read-the-grounding-rules-skill-on-10-15-2026"
)

// MCPClient speaks Streamable HTTP JSON-RPC 2.0 to https://mcp.quran.ai.
type MCPClient struct {
	BaseURL   string
	HTTP      *http.Client
	sessionID string
	reqID     atomic.Int64
}

func newMCPClient() *MCPClient {
	return &MCPClient{
		BaseURL: mcpBaseURL,
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type mcpToolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError           bool            `json:"isError"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

func (c *MCPClient) ensureSession(ctx context.Context) error {
	if c.sessionID != "" {
		return nil
	}
	id := c.reqID.Add(1)
	payload := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "quranctl",
				"version": "1.0.0",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", userAgentString)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp initialize status %d", resp.StatusCode)
	}
	sid := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id"))
	if sid == "" {
		return errors.New("mcp initialize: missing Mcp-Session-Id header")
	}
	c.sessionID = sid
	return nil
}

// CallTool invokes an MCP tool on https://mcp.quran.ai and unmarshals its structured
// JSON payload into out.
func (c *MCPClient) CallTool(ctx context.Context, toolName string, args map[string]any, out any) error {
	if err := c.ensureSession(ctx); err != nil {
		return err
	}
	id := c.reqID.Add(1)
	payload := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", c.sessionID)
	req.Header.Set("User-Agent", userAgentString)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp %s status %d: %s", toolName, resp.StatusCode, strings.TrimSpace(string(rawResp)))
	}

	jsonBytes := extractJSONFromMCPBody(rawResp)
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(jsonBytes, &rpcResp); err != nil {
		return fmt.Errorf("mcp %s parse rpc: %w", toolName, err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("mcp %s error %d: %s", toolName, rpcResp.Error.Code, rpcResp.Error.Message)
	}

	var toolRes mcpToolCallResult
	if err := json.Unmarshal(rpcResp.Result, &toolRes); err != nil {
		return fmt.Errorf("mcp %s parse result: %w", toolName, err)
	}
	if toolRes.IsError {
		msg := "tool error"
		if len(toolRes.Content) > 0 && toolRes.Content[0].Text != "" {
			msg = toolRes.Content[0].Text
		}
		return fmt.Errorf("mcp %s: %s", toolName, msg)
	}

	if len(toolRes.StructuredContent) > 0 && string(toolRes.StructuredContent) != "null" {
		return json.Unmarshal(toolRes.StructuredContent, out)
	}
	if len(toolRes.Content) > 0 && toolRes.Content[0].Text != "" {
		return json.Unmarshal([]byte(toolRes.Content[0].Text), out)
	}
	return fmt.Errorf("mcp %s: empty tool result", toolName)
}

func extractJSONFromMCPBody(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var lastData []byte
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if strings.HasPrefix(d, "{") {
				lastData = []byte(d)
			}
		}
	}
	if len(lastData) > 0 {
		return lastData
	}
	return trimmed
}

// SearchResultItem is returned by `quranctl search` and `quranctl navigate`.
type SearchResultItem struct {
	AyahKey     string  `json:"ayah_key"`
	Surah       int     `json:"surah"`
	SurahName   string  `json:"surah_name"`
	SurahArabic string  `json:"surah_arabic"`
	Ayah        int     `json:"ayah"`
	TotalVerses int     `json:"total_verses"`
	Arabic      string  `json:"arabic"`
	Translation string  `json:"translation"`
	Edition     string  `json:"edition,omitempty"`
	TimestampMs int64   `json:"timestamp_ms"`
	DurationMs  int64   `json:"duration_ms"`
	Juz         int     `json:"juz"`
	Page        int     `json:"page"`
	CitationURL string  `json:"citation_url,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

type SearchResponse struct {
	Query   string             `json:"query"`
	Mode    string             `json:"mode"` // "semantic" | "ayah" | "juz" | "page" | "hizb"
	Results []SearchResultItem `json:"results"`
}

// AyahDetailResponse is returned by `quranctl ayah`.
type AyahDetailResponse struct {
	AyahKey       string `json:"ayah_key"`
	Surah         int    `json:"surah"`
	SurahName     string `json:"surah_name"`
	SurahArabic   string `json:"surah_arabic"`
	Ayah          int    `json:"ayah"`
	TotalVerses   int    `json:"total_verses"`
	Arabic        string `json:"arabic"`
	Translation   string `json:"translation"`
	Edition       string `json:"edition"`
	Tafsir        string `json:"tafsir,omitempty"`
	TafsirEdition string `json:"tafsir_edition,omitempty"`
	Juz           int    `json:"juz"`
	Hizb          int    `json:"hizb,omitempty"`
	Page          int    `json:"page"`
	TimestampMs   int64  `json:"timestamp_ms"`
	DurationMs    int64  `json:"duration_ms"`
	QuranComURL   string `json:"quran_com_url"`
	StudyURL      string `json:"study_url"`
}

var (
	reJuzQuery  = regexp.MustCompile(`(?i)^(?:juz|j|para)\s*[:#-]?\s*(\d{1,2})$`)
	rePageQuery = regexp.MustCompile(`(?i)^(?:page|pg|p)\s*[:#-]?\s*(\d{1,3})$`)
	reHizbQuery = regexp.MustCompile(`(?i)^(?:hizb|h)\s*[:#-]?\s*(\d{1,2})$`)
	reAyahQuery = regexp.MustCompile(`^(\d{1,3})\s*[:/]\s*(\d{1,3})$`)
	reHTMLTag   = regexp.MustCompile(`<[^>]+>`)
	reMultiNL   = regexp.MustCompile(`\n{3,}`)
)

// cmdSearch implements `quranctl search <query> [--surah N] [--edition ID] [--limit N]`.
func cmdSearch(args []string, stdout, stderr io.Writer) int {
	var (
		surahFilter int
		edition     = defaultTranslationEdition
		limit       = 8
		queryParts  []string
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--surah":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl search: --surah requires a number")
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 114 {
				fmt.Fprintln(stderr, "quranctl search: invalid --surah")
				return 2
			}
			surahFilter = n
		case "--edition":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl search: --edition requires an id")
				return 2
			}
			i++
			edition = args[i]
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl search: --limit requires a number")
				return 2
			}
			i++
			if n, err := strconv.Atoi(args[i]); err == nil && n > 0 && n <= 25 {
				limit = n
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "quranctl search: unknown flag %q\n", args[i])
				return 2
			}
			queryParts = append(queryParts, args[i])
		}
	}

	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		fmt.Fprintln(stderr, "usage: quranctl search <query> [--surah N] [--edition ID] [--limit N]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	mcp := newMCPClient()
	timingsCache := map[int]map[int][2]int64{}

	// 1. Check if query is a Juz jump (e.g. "juz 30", "juz:30")
	if m := reJuzQuery.FindStringSubmatch(query); len(m) == 2 {
		juzNum, _ := strconv.Atoi(m[1])
		if juzNum >= 1 && juzNum <= 30 {
			item, err := navigateBySpan(ctx, mcp, "juz", juzNum, edition, timingsCache)
			if err != nil {
				fmt.Fprintf(stderr, "quranctl search: %v\n", err)
				return 1
			}
			writeJSON(stdout, SearchResponse{Query: query, Mode: "juz", Results: []SearchResultItem{item}})
			return 0
		}
	}

	// 2. Check if query is a Page jump (e.g. "page 50", "p 604")
	if m := rePageQuery.FindStringSubmatch(query); len(m) == 2 {
		pageNum, _ := strconv.Atoi(m[1])
		if pageNum >= 1 && pageNum <= 604 {
			item, err := navigateBySpan(ctx, mcp, "page", pageNum, edition, timingsCache)
			if err != nil {
				fmt.Fprintf(stderr, "quranctl search: %v\n", err)
				return 1
			}
			writeJSON(stdout, SearchResponse{Query: query, Mode: "page", Results: []SearchResultItem{item}})
			return 0
		}
	}

	// 3. Check if query is a Hizb jump (e.g. "hizb 5")
	if m := reHizbQuery.FindStringSubmatch(query); len(m) == 2 {
		hizbNum, _ := strconv.Atoi(m[1])
		if hizbNum >= 1 && hizbNum <= 60 {
			item, err := navigateBySpan(ctx, mcp, "hizb", hizbNum, edition, timingsCache)
			if err != nil {
				fmt.Fprintf(stderr, "quranctl search: %v\n", err)
				return 1
			}
			writeJSON(stdout, SearchResponse{Query: query, Mode: "hizb", Results: []SearchResultItem{item}})
			return 0
		}
	}

	// 4. Check if query is an exact verse reference (e.g. "2:255")
	if m := reAyahQuery.FindStringSubmatch(query); len(m) == 3 {
		sNum, _ := strconv.Atoi(m[1])
		aNum, _ := strconv.Atoi(m[2])
		if sNum >= 1 && sNum <= 114 {
			meta := surahMetadata[sNum]
			if aNum >= 1 && aNum <= meta.TotalVerses {
				detail, err := fetchAyahDetail(ctx, mcp, sNum, aNum, edition, "", false, timingsCache)
				if err != nil {
					fmt.Fprintf(stderr, "quranctl search: %v\n", err)
					return 1
				}
				item := SearchResultItem{
					AyahKey:     detail.AyahKey,
					Surah:       detail.Surah,
					SurahName:   detail.SurahName,
					SurahArabic: detail.SurahArabic,
					Ayah:        detail.Ayah,
					TotalVerses: detail.TotalVerses,
					Arabic:      detail.Arabic,
					Translation: detail.Translation,
					Edition:     detail.Edition,
					TimestampMs: detail.TimestampMs,
					DurationMs:  detail.DurationMs,
					Juz:         detail.Juz,
					Page:        detail.Page,
					CitationURL: detail.QuranComURL,
					Score:       1.0,
				}
				writeJSON(stdout, SearchResponse{Query: query, Mode: "ayah", Results: []SearchResultItem{item}})
				return 0
			}
		}
	}

	// 5. Semantic search via mcp.quran.ai `search_quran`
	argsMap := map[string]any{
		"query":           query,
		"translations":    edition,
		"intent":          "identify-ayah",
		"grounding_nonce": mcpGroundingNonce,
	}
	if surahFilter >= 1 && surahFilter <= 114 {
		argsMap["surah"] = surahFilter
	}

	var rawSearch struct {
		Results []struct {
			Rank         int     `json:"rank"`
			AyahKey      string  `json:"ayah_key"`
			Surah        int     `json:"surah"`
			Ayah         int     `json:"ayah"`
			Text         string  `json:"text"`
			CitationURL  string  `json:"citation_url"`
			Score        float64 `json:"relevance_score"`
			Translations []struct {
				Text    string `json:"text"`
				Edition struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Author string `json:"author"`
				} `json:"edition"`
			} `json:"translations"`
		} `json:"results"`
	}

	if err := mcp.CallTool(ctx, "search_quran", argsMap, &rawSearch); err != nil {
		fmt.Fprintf(stderr, "quranctl search: %v\n", err)
		return 1
	}

	results := make([]SearchResultItem, 0, limit)
	for _, r := range rawSearch.Results {
		if len(results) >= limit {
			break
		}
		sNum := r.Surah
		aNum := r.Ayah
		if (sNum == 0 || aNum == 0) && r.AyahKey != "" {
			sNum, aNum = parseAyahKey(r.AyahKey)
		}
		if sNum < 1 || sNum > 114 {
			continue
		}
		if aNum < 1 {
			aNum = 1
		}
		meta := surahMetadata[sNum]
		var trText, trEdition string
		if len(r.Translations) > 0 {
			trText = strings.TrimSpace(r.Translations[0].Text)
			trEdition = r.Translations[0].Edition.Name
			if r.Translations[0].Edition.Author != "" {
				trEdition = r.Translations[0].Edition.Author
			}
		}
		posMs, durMs := lookupVerseTiming(ctx, sNum, aNum, meta.TotalVerses, timingsCache)
		citation := r.CitationURL
		if citation == "" {
			citation = fmt.Sprintf("https://quran.com/%d:%d", sNum, aNum)
		}
		results = append(results, SearchResultItem{
			AyahKey:     fmt.Sprintf("%d:%d", sNum, aNum),
			Surah:       sNum,
			SurahName:   meta.Transliteration,
			SurahArabic: meta.Arabic,
			Ayah:        aNum,
			TotalVerses: meta.TotalVerses,
			Arabic:      strings.TrimSpace(r.Text),
			Translation: trText,
			Edition:     trEdition,
			TimestampMs: posMs,
			DurationMs:  durMs,
			Juz:         meta.Juz,
			Page:        meta.Page,
			CitationURL: citation,
			Score:       r.Score,
		})
	}

	writeJSON(stdout, SearchResponse{
		Query:   query,
		Mode:    "semantic",
		Results: results,
	})
	return 0
}

// cmdAyah implements `quranctl ayah <surah:ayah> [--edition ID] [--tafsir ID] [--with-tafsir]`.
func cmdAyah(args []string, stdout, stderr io.Writer) int {
	var (
		edition       = defaultTranslationEdition
		tafsirEdition = defaultTafsirEdition
		withTafsir    bool
		refArg        string
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--edition":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl ayah: --edition requires an id")
				return 2
			}
			i++
			edition = args[i]
		case "--tafsir":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl ayah: --tafsir requires an id")
				return 2
			}
			i++
			tafsirEdition = args[i]
			withTafsir = true
		case "--with-tafsir":
			withTafsir = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "quranctl ayah: unknown flag %q\n", args[i])
				return 2
			}
			if refArg == "" {
				refArg = args[i]
			}
		}
	}

	sNum, aNum := parseAyahKey(refArg)
	if sNum < 1 || sNum > 114 || aNum < 1 {
		fmt.Fprintln(stderr, "usage: quranctl ayah <surah:ayah> [--edition ID] [--tafsir ID] [--with-tafsir]")
		return 2
	}
	meta := surahMetadata[sNum]
	if aNum > meta.TotalVerses && meta.TotalVerses > 0 {
		aNum = meta.TotalVerses
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	mcp := newMCPClient()
	timingsCache := map[int]map[int][2]int64{}
	detail, err := fetchAyahDetail(ctx, mcp, sNum, aNum, edition, tafsirEdition, withTafsir, timingsCache)
	if err != nil {
		fmt.Fprintf(stderr, "quranctl ayah: %v\n", err)
		return 1
	}
	writeJSON(stdout, detail)
	return 0
}

// cmdNavigate implements `quranctl navigate [--juz N | --page N | --hizb N] [--edition ID]`.
func cmdNavigate(args []string, stdout, stderr io.Writer) int {
	var (
		spanKind string
		spanVal  int
		edition  = defaultTranslationEdition
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--juz":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl navigate: --juz requires 1..30")
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 30 {
				fmt.Fprintln(stderr, "quranctl navigate: --juz must be 1..30")
				return 2
			}
			spanKind = "juz"
			spanVal = n
		case "--page":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl navigate: --page requires 1..604")
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 604 {
				fmt.Fprintln(stderr, "quranctl navigate: --page must be 1..604")
				return 2
			}
			spanKind = "page"
			spanVal = n
		case "--hizb":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl navigate: --hizb requires 1..60")
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 60 {
				fmt.Fprintln(stderr, "quranctl navigate: --hizb must be 1..60")
				return 2
			}
			spanKind = "hizb"
			spanVal = n
		case "--edition":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl navigate: --edition requires an id")
				return 2
			}
			i++
			edition = args[i]
		default:
			fmt.Fprintf(stderr, "quranctl navigate: unknown argument %q\n", args[i])
			return 2
		}
	}
	if spanKind == "" {
		fmt.Fprintln(stderr, "usage: quranctl navigate {--juz 1..30 | --page 1..604 | --hizb 1..60}")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	mcp := newMCPClient()
	timingsCache := map[int]map[int][2]int64{}
	item, err := navigateBySpan(ctx, mcp, spanKind, spanVal, edition, timingsCache)
	if err != nil {
		fmt.Fprintf(stderr, "quranctl navigate: %v\n", err)
		return 1
	}
	writeJSON(stdout, item)
	return 0
}

func navigateBySpan(ctx context.Context, mcp *MCPClient, spanKind string, spanVal int, edition string, timingsCache map[int]map[int][2]int64) (SearchResultItem, error) {
	var metaResp struct {
		Ayah struct {
			StartVerseKey string `json:"start_verse_key"`
			EndVerseKey   string `json:"end_verse_key"`
		} `json:"ayah"`
		Juz struct {
			Start int `json:"start"`
		} `json:"juz"`
		Page struct {
			Start int `json:"start"`
		} `json:"page"`
	}
	if err := mcp.CallTool(ctx, "fetch_quran_metadata", map[string]any{spanKind: spanVal}, &metaResp); err != nil {
		return SearchResultItem{}, err
	}
	sNum, aNum := parseAyahKey(metaResp.Ayah.StartVerseKey)
	if sNum < 1 || sNum > 114 {
		return SearchResultItem{}, fmt.Errorf("invalid start_verse_key %q for %s %d", metaResp.Ayah.StartVerseKey, spanKind, spanVal)
	}
	if aNum < 1 {
		aNum = 1
	}
	detail, err := fetchAyahDetail(ctx, mcp, sNum, aNum, edition, "", false, timingsCache)
	if err != nil {
		return SearchResultItem{}, err
	}
	juz := detail.Juz
	if metaResp.Juz.Start > 0 {
		juz = metaResp.Juz.Start
	}
	page := detail.Page
	if metaResp.Page.Start > 0 {
		page = metaResp.Page.Start
	}
	return SearchResultItem{
		AyahKey:     detail.AyahKey,
		Surah:       detail.Surah,
		SurahName:   detail.SurahName,
		SurahArabic: detail.SurahArabic,
		Ayah:        detail.Ayah,
		TotalVerses: detail.TotalVerses,
		Arabic:      detail.Arabic,
		Translation: detail.Translation,
		Edition:     detail.Edition,
		TimestampMs: detail.TimestampMs,
		DurationMs:  detail.DurationMs,
		Juz:         juz,
		Page:        page,
		CitationURL: detail.QuranComURL,
	}, nil
}

func fetchAyahDetail(ctx context.Context, mcp *MCPClient, surah, ayah int, edition, tafsirEdition string, withTafsir bool, timingsCache map[int]map[int][2]int64) (AyahDetailResponse, error) {
	ayahKey := fmt.Sprintf("%d:%d", surah, ayah)
	meta := surahMetadata[surah]

	arabicMap, err := fetchMCPVersesMap(ctx, mcp, "fetch_quran", []string{ayahKey}, defaultArabicEdition)
	if err != nil {
		// Fallback to ar-simple-clean if ar-uthmani fails
		arabicMap, _ = fetchMCPVersesMap(ctx, mcp, "fetch_quran", []string{ayahKey}, "ar-simple-clean")
	}
	transMap, _ := fetchMCPVersesMap(ctx, mcp, "fetch_translation", []string{ayahKey}, edition)

	var metaResp struct {
		Juz struct {
			Start int `json:"start"`
		} `json:"juz"`
		Hizb struct {
			Start int `json:"start"`
		} `json:"hizb"`
		Page struct {
			Start int `json:"start"`
		} `json:"page"`
	}
	_ = mcp.CallTool(ctx, "fetch_quran_metadata", map[string]any{
		"surah": surah,
		"ayah":  ayah,
	}, &metaResp)

	juz := meta.Juz
	if metaResp.Juz.Start > 0 {
		juz = metaResp.Juz.Start
	}
	page := meta.Page
	if metaResp.Page.Start > 0 {
		page = metaResp.Page.Start
	}
	hizb := metaResp.Hizb.Start

	var tafsirText string
	if withTafsir {
		if tafsirEdition == "" {
			tafsirEdition = defaultTafsirEdition
		}
		tafsirText, _ = fetchMCPTafsirText(ctx, mcp, ayahKey, tafsirEdition)
	}

	posMs, durMs := lookupVerseTiming(ctx, surah, ayah, meta.TotalVerses, timingsCache)

	return AyahDetailResponse{
		AyahKey:       ayahKey,
		Surah:         surah,
		SurahName:     meta.Transliteration,
		SurahArabic:   meta.Arabic,
		Ayah:          ayah,
		TotalVerses:   meta.TotalVerses,
		Arabic:        arabicMap[ayahKey],
		Translation:   transMap[ayahKey],
		Edition:       edition,
		Tafsir:        tafsirText,
		TafsirEdition: tafsirEdition,
		Juz:           juz,
		Hizb:          hizb,
		Page:          page,
		TimestampMs:   posMs,
		DurationMs:    durMs,
		QuranComURL:   fmt.Sprintf("https://quran.com/%d/%d", surah, ayah),
		StudyURL:      fmt.Sprintf("https://quran.com/%d/%d/tafsirs", surah, ayah),
	}, nil
}

func fetchMCPVersesMap(ctx context.Context, mcp *MCPClient, toolName string, ayahKeys []string, edition string) (map[string]string, error) {
	out := map[string]string{}
	if len(ayahKeys) == 0 {
		return out, nil
	}
	var resp struct {
		Results map[string][]struct {
			Ayah string `json:"ayah"`
			Text string `json:"text"`
		} `json:"results"`
	}
	args := map[string]any{
		"ayahs":           ayahKeys,
		"grounding_nonce": mcpGroundingNonce,
	}
	if edition != "" {
		args["editions"] = edition
	}
	if err := mcp.CallTool(ctx, toolName, args, &resp); err != nil {
		return out, err
	}
	for _, list := range resp.Results {
		for _, item := range list {
			if item.Ayah != "" && item.Text != "" {
				out[item.Ayah] = strings.TrimSpace(item.Text)
			}
		}
	}
	return out, nil
}

func fetchMCPTafsirText(ctx context.Context, mcp *MCPClient, ayahKey, edition string) (string, error) {
	var resp struct {
		Results map[string][]struct {
			Range string `json:"range"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	err := mcp.CallTool(ctx, "fetch_tafsir", map[string]any{
		"ayahs":           ayahKey,
		"editions":        edition,
		"grounding_nonce": mcpGroundingNonce,
	}, &resp)
	if err != nil {
		return "", err
	}
	for _, list := range resp.Results {
		for _, item := range list {
			if item.Text != "" {
				return cleanHTMLText(item.Text), nil
			}
		}
	}
	return "", nil
}

// enrichBookmarksWithMCP populates ArabicText and Translation on bookmarks that
// are missing them by querying https://mcp.quran.ai in a single batch.
func enrichBookmarksWithMCP(ctx context.Context, bms []LocalBookmark) {
	if len(bms) == 0 {
		return
	}
	seen := map[string]bool{}
	var keys []string
	for _, bm := range bms {
		if bm.Surah < 1 || bm.Surah > 114 || bm.Ayah < 1 {
			continue
		}
		if bm.ArabicText != "" && bm.Translation != "" {
			continue
		}
		k := fmt.Sprintf("%d:%d", bm.Surah, bm.Ayah)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	// Cap batch to 25 keys at a time
	if len(keys) > 25 {
		keys = keys[:25]
	}
	mcp := newMCPClient()
	arabicMap, err := fetchMCPVersesMap(ctx, mcp, "fetch_quran", keys, defaultArabicEdition)
	if err != nil || len(arabicMap) == 0 {
		arabicMap, _ = fetchMCPVersesMap(ctx, mcp, "fetch_quran", keys, "ar-simple-clean")
	}
	transMap, _ := fetchMCPVersesMap(ctx, mcp, "fetch_translation", keys, defaultTranslationEdition)

	for i := range bms {
		k := fmt.Sprintf("%d:%d", bms[i].Surah, bms[i].Ayah)
		if bms[i].ArabicText == "" && arabicMap[k] != "" {
			bms[i].ArabicText = arabicMap[k]
		}
		if bms[i].Translation == "" && transMap[k] != "" {
			bms[i].Translation = transMap[k]
		}
	}
}

func parseAyahKey(s string) (int, int) {
	s = strings.TrimSpace(s)
	m := reAyahQuery.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, 0
	}
	surah, _ := strconv.Atoi(m[1])
	ayah, _ := strconv.Atoi(m[2])
	return surah, ayah
}

func cleanHTMLText(s string) string {
	s = strings.ReplaceAll(s, "</p>", "\n\n")
	s = strings.ReplaceAll(s, "</h1>", "\n\n")
	s = strings.ReplaceAll(s, "</h2>", "\n\n")
	s = strings.ReplaceAll(s, "</h3>", "\n\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func writeJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(w, string(b))
}
