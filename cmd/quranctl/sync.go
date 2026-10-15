// sync.go — `quranctl sync` and `quranctl login` for syncing bookmarks from Quran.com.
//
// Supports two authentication flows:
//  1. Quran.com Web Session flow (default, zero Developer Console setup):
//     Opens a local login page where the user clicks "Continue with Google /
//     Facebook / Apple" on Quran.com, receives the one-time redirect token at
//     http://127.0.0.1:19450/callback, exchanges it at
//     GET https://quran.com/api/proxy/auth/tokens/redirectToken, and stores the
//     session cookies in ~/.local/state/omarchy/quran/qf_auth.json (0600).
//  2. Official Quran Foundation OAuth2 + PKCE flow (when --client-id or
//     QF_CLIENT_ID is provided):
//     Uses https://oauth2.quran.foundation/oauth2/auth with S256 PKCE and fetches
//     bookmarks from https://apis.quran.foundation/auth/v1/bookmarks.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultCallbackAddr = "127.0.0.1:19450"
	quranComBaseURL     = "https://quran.com"
	qfOAuthBaseURL      = "https://oauth2.quran.foundation"
	qfAPIBaseURL        = "https://apis.quran.foundation"
	publicQuranAPIURL   = "https://api.quran.com"
	userAgentString     = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"
)

// AuthConfig is persisted at ~/.local/state/omarchy/quran/qf_auth.json (mode 0600).
type AuthConfig struct {
	Mode         string            `json:"mode"` // "web" or "oauth2"
	Cookies      map[string]string `json:"cookies,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	ClientSecret string            `json:"client_secret,omitempty"`
	AccessToken  string            `json:"access_token,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	ExpiresAt    int64             `json:"expires_at,omitempty"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
}

// LocalBookmark matches the schema in Service.qml (~/.local/state/omarchy/quran/bookmarks.json).
type LocalBookmark struct {
	ID          string `json:"id"`
	RemoteID    string `json:"remoteId,omitempty"`
	Surah       int    `json:"surah"`
	SurahName   string `json:"surahName"`
	SurahArabic string `json:"surahArabic"`
	Ayah        int    `json:"ayah"`
	TotalVerses int    `json:"totalVerses"`
	Juz         int    `json:"juz"`
	Page        int    `json:"page"`
	ReciterID   string `json:"reciterId"`
	Reciter     string `json:"reciter"`
	TimestampMs int64  `json:"timestamp_ms"`
	DurationMs  int64  `json:"duration_ms"`
	ArabicText  string `json:"arabicText,omitempty"`
	Translation string `json:"translation,omitempty"`
	Note        string `json:"note"`
	CreatedAt   string `json:"createdAt"`
}

type remoteBookmark struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	Type        string `json:"type"`
	Key         int    `json:"key"`
	VerseNumber int    `json:"verseNumber"`
	IsReading   *bool  `json:"isReading"`
}

type surahMeta struct {
	Transliteration string
	Arabic          string
	TotalVerses     int
	Juz             int
	Page            int
}

var surahMetadata = [115]surahMeta{
	0:   {},
	1:   {"Al-Fatihah", "الفاتحة", 7, 1, 1},
	2:   {"Al-Baqarah", "البقرة", 286, 1, 2},
	3:   {"Ali 'Imran", "آل عمران", 200, 3, 50},
	4:   {"An-Nisa", "النساء", 176, 4, 77},
	5:   {"Al-Ma'idah", "المائدة", 120, 6, 106},
	6:   {"Al-An'am", "الأنعام", 165, 7, 128},
	7:   {"Al-A'raf", "الأعراف", 206, 8, 151},
	8:   {"Al-Anfal", "الأنفال", 75, 9, 177},
	9:   {"At-Tawbah", "التوبة", 129, 10, 187},
	10:  {"Yunus", "يونس", 109, 11, 208},
	11:  {"Hud", "هود", 123, 11, 221},
	12:  {"Yusuf", "يوسف", 111, 12, 235},
	13:  {"Ar-Ra'd", "الرعد", 43, 13, 249},
	14:  {"Ibrahim", "ابراهيم", 52, 13, 255},
	15:  {"Al-Hijr", "الحجر", 99, 14, 262},
	16:  {"An-Nahl", "النحل", 128, 14, 267},
	17:  {"Al-Isra", "الإسراء", 111, 15, 282},
	18:  {"Al-Kahf", "الكهف", 110, 15, 293},
	19:  {"Maryam", "مريم", 98, 16, 305},
	20:  {"Taha", "طه", 135, 16, 312},
	21:  {"Al-Anbya", "الأنبياء", 112, 17, 322},
	22:  {"Al-Hajj", "الحج", 78, 17, 332},
	23:  {"Al-Mu'minun", "المؤمنون", 118, 18, 342},
	24:  {"An-Nur", "النور", 64, 18, 350},
	25:  {"Al-Furqan", "الفرقان", 77, 18, 359},
	26:  {"Ash-Shu'ara", "الشعراء", 227, 19, 367},
	27:  {"An-Naml", "النمل", 93, 19, 377},
	28:  {"Al-Qasas", "القصص", 88, 20, 385},
	29:  {"Al-'Ankabut", "العنكبوت", 69, 20, 396},
	30:  {"Ar-Rum", "الروم", 60, 21, 404},
	31:  {"Luqman", "لقمان", 34, 21, 411},
	32:  {"As-Sajdah", "السجدة", 30, 21, 415},
	33:  {"Al-Ahzab", "الأحزاب", 73, 21, 418},
	34:  {"Saba", "سبإ", 54, 22, 428},
	35:  {"Fatir", "فاطر", 45, 22, 434},
	36:  {"Ya-Sin", "يس", 83, 22, 440},
	37:  {"As-Saffat", "الصافات", 182, 23, 446},
	38:  {"Sad", "ص", 88, 23, 453},
	39:  {"Az-Zumar", "الزمر", 75, 23, 458},
	40:  {"Ghafir", "غافر", 85, 24, 467},
	41:  {"Fussilat", "فصلت", 54, 24, 477},
	42:  {"Ash-Shuraa", "الشورى", 53, 25, 483},
	43:  {"Az-Zukhruf", "الزخرف", 89, 25, 489},
	44:  {"Ad-Dukhan", "الدخان", 59, 25, 496},
	45:  {"Al-Jathiyah", "الجاثية", 37, 25, 499},
	46:  {"Al-Ahqaf", "الأحقاف", 35, 26, 502},
	47:  {"Muhammad", "محمد", 38, 26, 507},
	48:  {"Al-Fath", "الفتح", 29, 26, 511},
	49:  {"Al-Hujurat", "الحجرات", 18, 26, 515},
	50:  {"Qaf", "ق", 45, 26, 518},
	51:  {"Adh-Dhariyat", "الذاريات", 60, 26, 520},
	52:  {"At-Tur", "الطور", 49, 27, 523},
	53:  {"An-Najm", "النجم", 62, 27, 526},
	54:  {"Al-Qamar", "القمر", 55, 27, 528},
	55:  {"Ar-Rahman", "الرحمن", 78, 27, 531},
	56:  {"Al-Waqi'ah", "الواقعة", 96, 27, 534},
	57:  {"Al-Hadid", "الحديد", 29, 27, 537},
	58:  {"Al-Mujadila", "المجادلة", 22, 28, 542},
	59:  {"Al-Hashr", "الحشر", 24, 28, 545},
	60:  {"Al-Mumtahanah", "الممتحنة", 13, 28, 549},
	61:  {"As-Saf", "الصف", 14, 28, 551},
	62:  {"Al-Jumu'ah", "الجمعة", 11, 28, 553},
	63:  {"Al-Munafiqun", "المنافقون", 11, 28, 554},
	64:  {"At-Taghabun", "التغابن", 18, 28, 556},
	65:  {"At-Talaq", "الطلاق", 12, 28, 558},
	66:  {"At-Tahrim", "التحريم", 12, 28, 560},
	67:  {"Al-Mulk", "الملك", 30, 29, 562},
	68:  {"Al-Qalam", "القلم", 52, 29, 564},
	69:  {"Al-Haqqah", "الحاقة", 52, 29, 566},
	70:  {"Al-Ma'arij", "المعارج", 44, 29, 568},
	71:  {"Nuh", "نوح", 28, 29, 570},
	72:  {"Al-Jinn", "الجن", 28, 29, 572},
	73:  {"Al-Muzzammil", "المزمل", 20, 29, 574},
	74:  {"Al-Muddaththir", "المدثر", 56, 29, 575},
	75:  {"Al-Qiyamah", "القيامة", 40, 29, 577},
	76:  {"Al-Insan", "الانسان", 31, 29, 578},
	77:  {"Al-Mursalat", "المرسلات", 50, 29, 580},
	78:  {"An-Naba", "النبإ", 40, 30, 582},
	79:  {"An-Nazi'at", "النازعات", 46, 30, 583},
	80:  {"'Abasa", "عبس", 42, 30, 585},
	81:  {"At-Takwir", "التكوير", 29, 30, 586},
	82:  {"Al-Infitar", "الإنفطار", 19, 30, 587},
	83:  {"Al-Mutaffifin", "المطففين", 36, 30, 587},
	84:  {"Al-Inshiqaq", "الإنشقاق", 25, 30, 589},
	85:  {"Al-Buruj", "البروج", 22, 30, 590},
	86:  {"At-Tariq", "الطارق", 17, 30, 591},
	87:  {"Al-A'la", "الأعلى", 19, 30, 591},
	88:  {"Al-Ghashiyah", "الغاشية", 26, 30, 592},
	89:  {"Al-Fajr", "الفجر", 30, 30, 593},
	90:  {"Al-Balad", "البلد", 20, 30, 594},
	91:  {"Ash-Shams", "الشمس", 15, 30, 595},
	92:  {"Al-Layl", "الليل", 21, 30, 595},
	93:  {"Ad-Duhaa", "الضحى", 11, 30, 596},
	94:  {"Ash-Sharh", "الشرح", 8, 30, 596},
	95:  {"At-Tin", "التين", 8, 30, 597},
	96:  {"Al-'Alaq", "العلق", 19, 30, 597},
	97:  {"Al-Qadr", "القدر", 5, 30, 598},
	98:  {"Al-Bayyinah", "البينة", 8, 30, 598},
	99:  {"Az-Zalzalah", "الزلزلة", 8, 30, 599},
	100: {"Al-'Adiyat", "العاديات", 11, 30, 599},
	101: {"Al-Qari'ah", "القارعة", 11, 30, 600},
	102: {"At-Takathur", "التكاثر", 8, 30, 600},
	103: {"Al-'Asr", "العصر", 3, 30, 601},
	104: {"Al-Humazah", "الهمزة", 9, 30, 601},
	105: {"Al-Fil", "الفيل", 5, 30, 601},
	106: {"Quraysh", "قريش", 4, 30, 602},
	107: {"Al-Ma'un", "الماعون", 7, 30, 602},
	108: {"Al-Kawthar", "الكوثر", 3, 30, 602},
	109: {"Al-Kafirun", "الكافرون", 6, 30, 603},
	110: {"An-Nasr", "النصر", 3, 30, 603},
	111: {"Al-Masad", "المسد", 5, 30, 603},
	112: {"Al-Ikhlas", "الإخلاص", 4, 30, 604},
	113: {"Al-Falaq", "الفلق", 5, 30, 604},
	114: {"An-Nas", "الناس", 6, 30, 604},
}

var newSyncHTTPClient = func() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var openBrowserFunc = func(targetURL string) error {
	cmd := exec.Command("xdg-open", targetURL)
	return cmd.Start()
}

func cmdSync(args []string, stdout, stderr io.Writer) int {
	return runSyncOrLogin(args, false, stdout, stderr)
}

func cmdLogin(args []string, stdout, stderr io.Writer) int {
	return runSyncOrLogin(args, true, stdout, stderr)
}

func runSyncOrLogin(args []string, forceLogin bool, stdout, stderr io.Writer) int {
	home := os.Getenv("HOME")
	if home == "" {
		fmt.Fprintln(stderr, "quranctl: HOME is not set")
		return 2
	}
	stateDir := filepath.Join(home, ".local", "state", "omarchy", "quran")
	settingsFile := filepath.Join(home, ".local", "state", "omarchy", "settings", "quran.json")
	interactive := forceLogin
	clientID := os.Getenv("QF_CLIENT_ID")
	clientSecret := os.Getenv("QF_CLIENT_SECRET")
	provider := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl: --state-dir requires a value")
				return 2
			}
			stateDir = args[i+1]
			i++
		case "--interactive", "-i":
			interactive = true
		case "--client-id":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl: --client-id requires a value")
				return 2
			}
			clientID = args[i+1]
			i++
		case "--client-secret":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl: --client-secret requires a value")
				return 2
			}
			clientSecret = args[i+1]
			i++
		case "--provider":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "quranctl: --provider requires a value (google|facebook|apple)")
				return 2
			}
			provider = strings.ToLower(args[i+1])
			i++
		default:
			fmt.Fprintf(stderr, "quranctl: unexpected argument %q\n", args[i])
			return 2
		}
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "quranctl: create state dir: %v\n", err)
		return 1
	}

	authPath := filepath.Join(stateDir, "qf_auth.json")
	bookmarksPath := filepath.Join(stateDir, "bookmarks.json")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	authCfg, _ := loadAuthConfig(authPath)
	if clientID != "" {
		authCfg.ClientID = clientID
		authCfg.Mode = "oauth2"
	}
	if clientSecret != "" {
		authCfg.ClientSecret = clientSecret
	}

	needLogin := forceLogin || !hasValidCredentials(authCfg)
	if !needLogin {
		added, total, err := performBookmarkSync(ctx, authCfg, authPath, bookmarksPath, settingsFile)
		if err == nil {
			writeSyncResult(stdout, added, total)
			return 0
		}
		if !interactive {
			fmt.Fprintf(stderr, "quranctl sync: %v (run 'quranctl login' to authenticate)\n", err)
			return 1
		}
	} else if !interactive {
		fmt.Fprintln(stderr, "quranctl sync: not authenticated (run 'quranctl login' or 'quranctl sync --interactive')")
		return 1
	}

	// Interactive login via local callback server
	newCfg, err := runInteractiveLogin(ctx, authCfg, provider, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "quranctl login: %v\n", err)
		return 1
	}
	if err := saveAuthConfig(authPath, newCfg); err != nil {
		fmt.Fprintf(stderr, "quranctl login: save auth: %v\n", err)
		return 1
	}

	added, total, err := performBookmarkSync(ctx, newCfg, authPath, bookmarksPath, settingsFile)
	if err != nil {
		fmt.Fprintf(stderr, "quranctl sync: %v\n", err)
		return 1
	}
	writeSyncResult(stdout, added, total)
	return 0
}

func writeSyncResult(w io.Writer, added, total int) {
	out, _ := json.Marshal(map[string]any{
		"status": "ok",
		"synced": added,
		"total":  total,
	})
	fmt.Fprintln(w, string(out))
}

func hasValidCredentials(cfg AuthConfig) bool {
	if cfg.Mode == "oauth2" && cfg.ClientID != "" && (cfg.AccessToken != "" || cfg.RefreshToken != "") {
		return true
	}
	if len(cfg.Cookies) > 0 && (cfg.Cookies["at"] != "" || cfg.Cookies["rt"] != "") {
		return true
	}
	return false
}

func loadAuthConfig(path string) (AuthConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return AuthConfig{Mode: "web", Cookies: map[string]string{}}, err
	}
	var cfg AuthConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return AuthConfig{Mode: "web", Cookies: map[string]string{}}, err
	}
	if cfg.Cookies == nil {
		cfg.Cookies = map[string]string{}
	}
	if cfg.Mode == "" {
		if cfg.ClientID != "" {
			cfg.Mode = "oauth2"
		} else {
			cfg.Mode = "web"
		}
	}
	return cfg, nil
}

func saveAuthConfig(path string, cfg AuthConfig) error {
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, b, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func runInteractiveLogin(ctx context.Context, cfg AuthConfig, provider string, stderr io.Writer) (AuthConfig, error) {
	ln, err := net.Listen("tcp", defaultCallbackAddr)
	if err != nil {
		return cfg, fmt.Errorf("listen on %s: %w", defaultCallbackAddr, err)
	}
	defer ln.Close()

	callbackURL := "http://" + ln.Addr().String() + "/callback"
	loginURL := "http://" + ln.Addr().String() + "/login"

	pkceVerifier, pkceChallenge, err := generatePKCE()
	if err != nil {
		return cfg, err
	}
	stateTok, err := randomHex(16)
	if err != nil {
		return cfg, err
	}
	nonceTok, err := randomHex(16)
	if err != nil {
		return cfg, err
	}

	resultCh := make(chan AuthConfig, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Mode == "oauth2" && cfg.ClientID != "" {
			u := buildOAuth2AuthorizeURL(qfOAuthBaseURL, cfg.ClientID, callbackURL, stateTok, nonceTok, pkceChallenge)
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
		p := strings.ToLower(r.URL.Query().Get("provider"))
		if p == "" {
			p = provider
		}
		if p == "google" || p == "facebook" || p == "apple" {
			loc, err := resolveSocialLoginRedirect(r.Context(), p, callbackURL)
			if err != nil {
				http.Error(w, "Failed to initiate "+p+" login: "+err.Error(), http.StatusBadGateway)
				return
			}
			http.Redirect(w, r, loc, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, renderLoginHTML())
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// 1. Quran.com Social Login Redirect Token flow (?token=...)
		if redirectTok := q.Get("token"); redirectTok != "" {
			cookies, err := exchangeQuranComRedirectToken(r.Context(), redirectTok)
			if err != nil {
				http.Error(w, "Authentication failed: "+err.Error(), http.StatusBadGateway)
				select {
				case errCh <- err:
				default:
				}
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, renderSuccessHTML())
			outCfg := cfg
			outCfg.Mode = "web"
			outCfg.Cookies = cookies
			select {
			case resultCh <- outCfg:
			default:
			}
			return
		}

		// 2. OAuth2 Authorization Code + PKCE flow (?code=...)
		if code := q.Get("code"); code != "" {
			if q.Get("state") != stateTok {
				http.Error(w, "Invalid OAuth2 state", http.StatusBadRequest)
				select {
				case errCh <- errors.New("invalid OAuth2 state on callback"):
				default:
				}
				return
			}
			outCfg, err := exchangeOAuth2Code(r.Context(), cfg, code, callbackURL, pkceVerifier)
			if err != nil {
				http.Error(w, "Token exchange failed: "+err.Error(), http.StatusBadGateway)
				select {
				case errCh <- err:
				default:
				}
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, renderSuccessHTML())
			select {
			case resultCh <- outCfg:
			default:
			}
			return
		}

		http.Error(w, "Missing token or code in callback", http.StatusBadRequest)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	fmt.Fprintf(stderr, "Opening browser for Quran.com login: %s\n", loginURL)
	_ = openBrowserFunc(loginURL)

	timer := time.NewTimer(3 * time.Minute)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return cfg, ctx.Err()
	case <-timer.C:
		return cfg, errors.New("login timed out after 3 minutes")
	case err := <-errCh:
		return cfg, err
	case newCfg := <-resultCh:
		return newCfg, nil
	}
}

func exchangeQuranComRedirectToken(ctx context.Context, redirectToken string) (map[string]string, error) {
	client := newSyncHTTPClient()
	endpoint := fmt.Sprintf("%s/api/proxy/auth/tokens/redirectToken?token=%s", quranComBaseURL, url.QueryEscape(redirectToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setQuranComHeaders(req, nil)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("redirectToken status %d", resp.StatusCode)
	}
	cookies := map[string]string{}
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			cookies[c.Name] = c.Value
		}
	}
	if cookies["at"] == "" && cookies["rt"] == "" {
		return nil, errors.New("no session cookies returned from redirectToken")
	}
	return cookies, nil
}

func refreshQuranComWebCookies(ctx context.Context, cookies map[string]string) (map[string]string, error) {
	client := newSyncHTTPClient()
	endpoint := quranComBaseURL + "/api/proxy/auth/tokens/refreshToken"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setQuranComHeaders(req, cookies)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refreshToken status %d", resp.StatusCode)
	}
	updated := make(map[string]string, len(cookies))
	for k, v := range cookies {
		updated[k] = v
	}
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			updated[c.Name] = c.Value
		}
	}
	return updated, nil
}

func buildOAuth2AuthorizeURL(baseURL, clientID, redirectURI, state, nonce, codeChallenge string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("scope", "openid offline_access bookmark collection user")
	v.Set("state", state)
	v.Set("nonce", nonce)
	v.Set("code_challenge", codeChallenge)
	v.Set("code_challenge_method", "S256")
	return strings.TrimRight(baseURL, "/") + "/oauth2/auth?" + v.Encode()
}

func exchangeOAuth2Code(ctx context.Context, cfg AuthConfig, code, redirectURI, verifier string) (AuthConfig, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	if cfg.ClientSecret == "" {
		form.Set("client_id", cfg.ClientID)
	}
	return doOAuth2TokenRequest(ctx, cfg, form)
}

func refreshOAuth2Token(ctx context.Context, cfg AuthConfig) (AuthConfig, error) {
	if cfg.RefreshToken == "" {
		return cfg, errors.New("missing OAuth2 refresh_token")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cfg.RefreshToken)
	if cfg.ClientSecret == "" {
		form.Set("client_id", cfg.ClientID)
	}
	return doOAuth2TokenRequest(ctx, cfg, form)
}

func doOAuth2TokenRequest(ctx context.Context, cfg AuthConfig, form url.Values) (AuthConfig, error) {
	client := newSyncHTTPClient()
	endpoint := qfOAuthBaseURL + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return cfg, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if cfg.ClientSecret != "" {
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return cfg, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return cfg, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return cfg, fmt.Errorf("oauth2 token endpoint status %d", resp.StatusCode)
	}
	var tokResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokResp); err != nil {
		return cfg, err
	}
	if tokResp.AccessToken == "" {
		return cfg, errors.New("empty access_token in OAuth2 response")
	}
	cfg.Mode = "oauth2"
	cfg.AccessToken = tokResp.AccessToken
	if tokResp.RefreshToken != "" {
		cfg.RefreshToken = tokResp.RefreshToken
	}
	if tokResp.ExpiresIn > 0 {
		cfg.ExpiresAt = time.Now().Unix() + tokResp.ExpiresIn
	}
	return cfg, nil
}

func performBookmarkSync(ctx context.Context, cfg AuthConfig, authPath, bookmarksPath, settingsPath string) (int, int, error) {
	remoteBms, updatedCfg, err := fetchRemoteBookmarksWithRefresh(ctx, cfg)
	if err != nil {
		return 0, 0, err
	}
	_ = saveAuthConfig(authPath, updatedCfg)

	reciterID, reciterName := readDefaultReciter(settingsPath)
	existing := loadLocalBookmarks(bookmarksPath)

	// Index existing bookmarks by remoteId and by surah:ayah to prevent duplicates
	byRemoteID := map[string]int{}
	bySurahAyah := map[string]int{}
	for idx, bm := range existing {
		if bm.RemoteID != "" {
			byRemoteID[bm.RemoteID] = idx
		}
		key := fmt.Sprintf("%d:%d", bm.Surah, bm.Ayah)
		if _, exists := bySurahAyah[key]; !exists {
			bySurahAyah[key] = idx
		}
	}

	// Cache chapter verse timings per surah so multiple bookmarks in the same surah only fetch once
	timingsCache := map[int]map[int][2]int64{}

	addedCount := 0
	var newItems []LocalBookmark

	for _, rb := range remoteBms {
		if rb.Key < 1 || rb.Key > 114 {
			continue
		}
		ayah := rb.VerseNumber
		if ayah < 1 {
			ayah = 1
		}
		meta := surahMetadata[rb.Key]
		if ayah > meta.TotalVerses && meta.TotalVerses > 0 {
			ayah = meta.TotalVerses
		}

		saKey := fmt.Sprintf("%d:%d", rb.Key, ayah)
		if idx, ok := byRemoteID[rb.ID]; ok && rb.ID != "" {
			if existing[idx].TimestampMs == 0 {
				posMs, durMs := lookupVerseTiming(ctx, rb.Key, ayah, meta.TotalVerses, timingsCache)
				existing[idx].TimestampMs = posMs
				existing[idx].DurationMs = durMs
			}
			continue
		}
		if idx, ok := bySurahAyah[saKey]; ok {
			if existing[idx].RemoteID == "" && rb.ID != "" {
				existing[idx].RemoteID = rb.ID
			}
			if existing[idx].TimestampMs == 0 {
				posMs, durMs := lookupVerseTiming(ctx, rb.Key, ayah, meta.TotalVerses, timingsCache)
				existing[idx].TimestampMs = posMs
				existing[idx].DurationMs = durMs
			}
			continue
		}

		posMs, durMs := lookupVerseTiming(ctx, rb.Key, ayah, meta.TotalVerses, timingsCache)
		note := fmt.Sprintf("Quran.com · Ayah %d", ayah)
		if rb.IsReading != nil && *rb.IsReading {
			note = fmt.Sprintf("Reading Bookmark · Ayah %d", ayah)
		}
		createdAt := rb.CreatedAt
		if createdAt == "" {
			createdAt = rb.UpdatedAt
		}
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}
		bmID := rb.ID
		if bmID == "" {
			bmID = fmt.Sprintf("qf-%d-%d", rb.Key, ayah)
		}

		localBm := LocalBookmark{
			ID:          bmID,
			RemoteID:    rb.ID,
			Surah:       rb.Key,
			SurahName:   meta.Transliteration,
			SurahArabic: meta.Arabic,
			Ayah:        ayah,
			TotalVerses: meta.TotalVerses,
			Juz:         meta.Juz,
			Page:        meta.Page,
			ReciterID:   reciterID,
			Reciter:     reciterName,
			TimestampMs: posMs,
			DurationMs:  durMs,
			Note:        note,
			CreatedAt:   createdAt,
		}
		newItems = append(newItems, localBm)
		bySurahAyah[saKey] = -1
		if rb.ID != "" {
			byRemoteID[rb.ID] = -1
		}
		addedCount++
	}

	merged := append(newItems, existing...)
	enrichBookmarksWithMCP(ctx, merged)
	outBytes, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	if err := writeFileAtomic(bookmarksPath, outBytes, 0o600); err != nil {
		return 0, 0, err
	}
	return addedCount, len(merged), nil
}

func fetchRemoteBookmarksWithRefresh(ctx context.Context, cfg AuthConfig) ([]remoteBookmark, AuthConfig, error) {
	if cfg.Mode == "oauth2" && cfg.ClientID != "" {
		if cfg.ExpiresAt > 0 && time.Now().Unix()+30 >= cfg.ExpiresAt && cfg.RefreshToken != "" {
			if refreshed, err := refreshOAuth2Token(ctx, cfg); err == nil {
				cfg = refreshed
			}
		}
		bms, status, err := fetchOAuth2Bookmarks(ctx, cfg)
		if (status == http.StatusUnauthorized || status == http.StatusForbidden) && cfg.RefreshToken != "" {
			refreshed, rErr := refreshOAuth2Token(ctx, cfg)
			if rErr != nil {
				return nil, cfg, rErr
			}
			cfg = refreshed
			bms, _, err = fetchOAuth2Bookmarks(ctx, cfg)
		}
		return bms, cfg, err
	}

	bms, status, err := fetchWebProxyBookmarks(ctx, cfg.Cookies)
	if (status == http.StatusUnauthorized || status == http.StatusForbidden) && cfg.Cookies["rt"] != "" {
		refreshedCookies, rErr := refreshQuranComWebCookies(ctx, cfg.Cookies)
		if rErr != nil {
			return nil, cfg, rErr
		}
		cfg.Cookies = refreshedCookies
		bms, _, err = fetchWebProxyBookmarks(ctx, cfg.Cookies)
	}
	return bms, cfg, err
}

func fetchWebProxyBookmarks(ctx context.Context, cookies map[string]string) ([]remoteBookmark, int, error) {
	client := newSyncHTTPClient()
	var all []remoteBookmark

	// 1. Saved + Reading bookmarks from /api/proxy/auth/bookmarks?mushafId=1
	bmsURL := quranComBaseURL + "/api/proxy/auth/bookmarks?mushafId=1"
	body, status, err := doGetWithCookies(ctx, client, bmsURL, cookies)
	if err != nil {
		return nil, status, err
	}
	parsed, err := parseBookmarksPayload(body)
	if err != nil {
		return nil, status, err
	}
	all = append(all, parsed...)

	// 2. Also check /api/proxy/auth/reading-bookmarks?mushafId=1
	readingURL := quranComBaseURL + "/api/proxy/auth/reading-bookmarks?mushafId=1"
	if rBody, rStatus, rErr := doGetWithCookies(ctx, client, readingURL, cookies); rErr == nil && rStatus == http.StatusOK {
		if rParsed, pErr := parseBookmarksPayload(rBody); pErr == nil {
			for i := range rParsed {
				t := true
				rParsed[i].IsReading = &t
			}
			all = append(all, rParsed...)
		}
	}

	return deduplicateRemoteBookmarks(all), http.StatusOK, nil
}

func fetchOAuth2Bookmarks(ctx context.Context, cfg AuthConfig) ([]remoteBookmark, int, error) {
	client := newSyncHTTPClient()
	var all []remoteBookmark

	bmsURL := qfAPIBaseURL + "/auth/v1/bookmarks?mushafId=1&type=ayah&first=50"
	body, status, err := doGetWithOAuth2(ctx, client, bmsURL, cfg)
	if err != nil {
		return nil, status, err
	}
	parsed, err := parseBookmarksPayload(body)
	if err != nil {
		return nil, status, err
	}
	all = append(all, parsed...)

	readingURL := qfAPIBaseURL + "/auth/v1/reading-bookmarks?mushafId=1"
	if rBody, rStatus, rErr := doGetWithOAuth2(ctx, client, readingURL, cfg); rErr == nil && rStatus == http.StatusOK {
		if rParsed, pErr := parseBookmarksPayload(rBody); pErr == nil {
			for i := range rParsed {
				t := true
				rParsed[i].IsReading = &t
			}
			all = append(all, rParsed...)
		}
	}

	return deduplicateRemoteBookmarks(all), http.StatusOK, nil
}

func doGetWithCookies(ctx context.Context, client *http.Client, targetURL string, cookies map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, err
	}
	setQuranComHeaders(req, cookies)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d from %s", resp.StatusCode, targetURL)
	}
	return body, resp.StatusCode, nil
}

func doGetWithOAuth2(ctx context.Context, client *http.Client, targetURL string, cfg AuthConfig) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("x-auth-token", cfg.AccessToken)
	req.Header.Set("x-client-id", cfg.ClientID)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgentString)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d from %s", resp.StatusCode, targetURL)
	}
	return body, resp.StatusCode, nil
}

func setQuranComHeaders(req *http.Request, cookies map[string]string) {
	req.Header.Set("Origin", quranComBaseURL)
	req.Header.Set("Referer", quranComBaseURL+"/")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgentString)
	if len(cookies) > 0 {
		var parts []string
		keys := make([]string, 0, len(cookies))
		for k := range cookies {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if cookies[k] != "" {
				parts = append(parts, k+"="+cookies[k])
			}
		}
		if len(parts) > 0 {
			req.Header.Set("Cookie", strings.Join(parts, "; "))
		}
	}
}

func parseBookmarksPayload(body []byte) ([]remoteBookmark, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []remoteBookmark
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	dataTrim := strings.TrimSpace(string(env.Data))
	if strings.HasPrefix(dataTrim, "[") {
		var list []remoteBookmark
		if err := json.Unmarshal(env.Data, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var nested struct {
		Bookmarks []remoteBookmark `json:"bookmarks"`
	}
	if err := json.Unmarshal(env.Data, &nested); err == nil && nested.Bookmarks != nil {
		return nested.Bookmarks, nil
	}
	return nil, nil
}

func deduplicateRemoteBookmarks(in []remoteBookmark) []remoteBookmark {
	seen := map[string]bool{}
	var out []remoteBookmark
	for _, b := range in {
		if b.Type != "" && b.Type != "ayah" {
			continue
		}
		if b.Key < 1 || b.Key > 114 {
			continue
		}
		ayah := b.VerseNumber
		if ayah < 1 {
			ayah = 1
		}
		k := fmt.Sprintf("%d:%d", b.Key, ayah)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, b)
	}
	return out
}

func lookupVerseTiming(ctx context.Context, surah, ayah, totalVerses int, cache map[int]map[int][2]int64) (int64, int64) {
	if m, ok := cache[surah]; ok {
		if pair, found := m[ayah]; found {
			return pair[0], pair[1]
		}
		return 0, 0
	}
	timings := map[int][2]int64{}
	cache[surah] = timings

	client := newSyncHTTPClient()
	u := fmt.Sprintf("%s/api/v4/chapter_recitations/7/%d?segments=true", publicQuranAPIURL, surah)
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return 0, 0
	}
	var payload struct {
		AudioFile struct {
			Timestamps []struct {
				VerseKey      string `json:"verse_key"`
				TimestampFrom int64  `json:"timestamp_from"`
				TimestampTo   int64  `json:"timestamp_to"`
			} `json:"timestamps"`
		} `json:"audio_file"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	var totalDur int64
	for _, ts := range payload.AudioFile.Timestamps {
		if ts.TimestampTo > totalDur {
			totalDur = ts.TimestampTo
		}
	}
	for _, ts := range payload.AudioFile.Timestamps {
		parts := strings.SplitN(ts.VerseKey, ":", 2)
		if len(parts) == 2 {
			if vNum, err := strconv.Atoi(parts[1]); err == nil {
				timings[vNum] = [2]int64{ts.TimestampFrom, totalDur}
			}
		}
	}
	if pair, ok := timings[ayah]; ok {
		return pair[0], pair[1]
	}
	return 0, totalDur
}

func readDefaultReciter(settingsPath string) (string, string) {
	reciterID := "ar.alafasy"
	reciterName := "Mishary Alafasi"
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		return reciterID, reciterName
	}
	var st struct {
		ReciterID string `json:"reciterId"`
		Reciters  []struct {
			Identifier  string `json:"identifier"`
			EnglishName string `json:"englishName"`
			Name        string `json:"name"`
		} `json:"reciters"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return reciterID, reciterName
	}
	if st.ReciterID != "" {
		reciterID = st.ReciterID
		for _, r := range st.Reciters {
			if r.Identifier == reciterID {
				if r.EnglishName != "" {
					reciterName = r.EnglishName
				} else if r.Name != "" {
					reciterName = r.Name
				}
				break
			}
		}
	}
	return reciterID, reciterName
}

func loadLocalBookmarks(path string) []LocalBookmark {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []LocalBookmark
	if err := json.Unmarshal(b, &list); err != nil {
		return nil
	}
	return list
}

func generatePKCE() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func resolveSocialLoginRedirect(ctx context.Context, provider, callbackURL string) (string, error) {
	u := quranComBaseURL + "/api/proxy/auth/auth/" + url.PathEscape(provider) + "?redirect=" + url.QueryEscape(callbackURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	setQuranComHeaders(req, nil)
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := strings.TrimSpace(resp.Header.Get("Location"))
		if loc != "" {
			return loc, nil
		}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return "", fmt.Errorf("unexpected status %d from Quran.com auth proxy: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func renderLoginHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Connect Quran.com — Omarchy Quran Plugin</title>
<style>
  body { background: #111318; color: #e5e9f0; font-family: system-ui, -apple-system, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
  .card { background: #1a1d24; border: 1px solid #2e3440; border-radius: 14px; padding: 32px; width: 360px; text-align: center; box-shadow: 0 12px 32px rgba(0,0,0,0.45); }
  h1 { font-size: 20px; margin: 0 0 8px; }
  p { font-size: 13px; color: #9aa5b4; margin: 0 0 24px; line-height: 1.5; }
  .btn { display: block; text-decoration: none; padding: 12px 16px; border-radius: 8px; font-weight: 600; font-size: 14px; margin-bottom: 10px; border: 1px solid #3b4252; color: #eceff4; background: #242933; transition: background 0.15s; }
  .btn:hover { background: #2e3440; }
  .btn-google { background: #2ca4ab; color: #0f1419; border-color: #2ca4ab; }
  .btn-google:hover { background: #36bac2; }
</style>
</head>
<body>
  <div class="card">
    <h1>Sync Quran.com Bookmarks</h1>
    <p>Sign in with your Quran.com account to sync your saved ayahs and reading bookmarks into the Omarchy Quran plugin.</p>
    <a class="btn btn-google" href="/login?provider=google">Continue with Google</a>
    <a class="btn" href="/login?provider=facebook">Continue with Facebook</a>
    <a class="btn" href="/login?provider=apple">Continue with Apple</a>
  </div>
</body>
</html>`
}

func renderSuccessHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Quran.com Connected</title>
<style>
  body { background: #111318; color: #e5e9f0; font-family: system-ui, -apple-system, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
  .card { background: #1a1d24; border: 1px solid #2ca4ab; border-radius: 14px; padding: 32px; width: 360px; text-align: center; box-shadow: 0 12px 32px rgba(0,0,0,0.45); }
  h1 { font-size: 20px; margin: 0 0 8px; color: #2ca4ab; }
  p { font-size: 13px; color: #9aa5b4; margin: 0; line-height: 1.5; }
</style>
</head>
<body>
  <div class="card">
    <h1>✓ Quran.com Connected</h1>
    <p>Your bookmarks are now syncing to the Omarchy Quran plugin. You can close this tab.</p>
  </div>
</body>
</html>`
}
