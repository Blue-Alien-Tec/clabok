package grammar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Language is one supported check language.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// LanguageList is a cached server language set.
type LanguageList struct {
	Endpoint  string     `json:"endpoint"`
	Fetched   time.Time  `json:"fetched"`
	Languages []Language `json:"languages"`
}

// LanguageCacheTTL is how long a cached list is used without a refresh.
const LanguageCacheTTL = 10 * time.Minute

const languageCacheFileName = "languages.json"

// languageCachePath lives in the grammar state directory, never in the store.
func languageCachePath() string { return filepath.Join(stateDir(), languageCacheFileName) }

// Languages fetches the supported set from the server.
func (client *Client) Languages(ctx context.Context) ([]Language, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL(client.Endpoint)+"/v2/languages", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return nil, classifyTransport(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &ErrResponse{Status: response.StatusCode}
	}
	var payload []struct {
		Code     string `json:"code"`
		LongCode string `json:"longCode"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, &ErrResponse{Status: response.StatusCode, Detail: err.Error()}
	}
	languages := make([]Language, 0, len(payload))
	for _, item := range payload {
		code := strings.TrimSpace(item.LongCode)
		if code == "" {
			code = strings.TrimSpace(item.Code)
		}
		if code == "" {
			continue
		}
		languages = append(languages, Language{Code: code, Name: strings.TrimSpace(item.Name)})
	}
	return languages, nil
}

// CachedLanguages returns the cached list for the configured endpoint.
func (client *Client) CachedLanguages(ctx context.Context) (LanguageList, bool) {
	list, ok := readLanguageCache()
	if !ok {
		return LanguageList{}, false
	}
	if !strings.EqualFold(strings.TrimRight(list.Endpoint, "/"),
		strings.TrimRight(client.Endpoint, "/")) {
		return LanguageList{}, false
	}
	return list, true
}

// Supported resolves a language id against the server set. The returned code is the server's
// spelling. A stale cache is used only when the server cannot answer; a miss refreshes once.
func (client *Client) Supported(ctx context.Context, code string) (Language, bool, error) {
	code = strings.TrimSpace(code)
	cached, hasCache := client.CachedLanguages(ctx)
	fresh := hasCache && time.Since(cached.Fetched) < LanguageCacheTTL
	if fresh {
		if language, ok := findLanguage(cached.Languages, code); ok {
			return language, true, nil
		}
	}
	languages, err := client.Languages(ctx)
	if err == nil {
		list := LanguageList{Endpoint: client.Endpoint, Fetched: time.Now().UTC(), Languages: languages}
		writeLanguageCache(list)
		language, ok := findLanguage(languages, code)
		return language, ok, nil
	}
	if hasCache {
		if language, ok := findLanguage(cached.Languages, code); ok {
			return language, true, nil
		}
	}
	return Language{}, false, err
}

// LanguageUnverified is the warning code of a language a book set could not verify because the
// server could not be reached.
const LanguageUnverified = "grammar.language-unverified"

// LanguageCheck validates a declared language against the server. It returns the server's spelling
// and no error when the id is supported, a grammar.language-unsupported error when the server
// reports a different set, and the code itself plus a warning when the server cannot be reached
// (LanguageTool stays optional).
func LanguageCheck(endpoint string, timeout time.Duration) func(code string) (string, string, error) {
	return func(code string) (string, string, error) {
		client := NewClientTimeout(endpoint, timeout)
		language, ok, err := client.Supported(context.Background(), strings.TrimSpace(code))
		if err == nil {
			if !ok {
				return "", "", &ServerError{
					Code: ReasonLanguageUnsupported,
					Message: fmt.Sprintf(
						"the server does not report the language %q", strings.TrimSpace(code)),
				}
			}
			return language.Code, "", nil
		}
		if ReasonFor(err) == ReasonUnreachable {
			return strings.TrimSpace(code), "the grammar server cannot be reached; " +
				"the language is unverified", nil
		}
		return "", "", &ServerError{Code: ReasonFor(err), Message: err.Error()}
	}
}

// languageSnapshot returns the list from the cache, refreshing it when requested, when the cache is
// missing or stale, or when the server answers. It never fails: an unreachable server leaves the
// cached list and reports it as stale.
func languageSnapshot(client *Client, refresh bool) ([]Language, bool, bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), client.Cap())
	defer cancel()
	cached, hasCache := client.CachedLanguages(ctx)
	stale := !hasCache || time.Since(cached.Fetched) >= LanguageCacheTTL
	if hasCache && !stale && !refresh {
		return cached.Languages, true, false, cached.Fetched.Format(time.RFC3339)
	}
	languages, err := client.Languages(ctx)
	if err == nil {
		list := LanguageList{Endpoint: client.Endpoint, Fetched: time.Now().UTC(), Languages: languages}
		writeLanguageCache(list)
		return list.Languages, false, false, list.Fetched.Format(time.RFC3339)
	}
	if hasCache {
		return cached.Languages, true, true, cached.Fetched.Format(time.RFC3339)
	}
	return nil, false, false, ""
}

func findLanguage(languages []Language, code string) (Language, bool) {
	for _, language := range languages {
		if strings.EqualFold(language.Code, code) {
			return language, true
		}
	}
	return Language{}, false
}

func readLanguageCache() (LanguageList, bool) {
	data, err := os.ReadFile(languageCachePath())
	if err != nil {
		return LanguageList{}, false
	}
	var list LanguageList
	if err := json.Unmarshal(data, &list); err != nil || len(list.Languages) == 0 {
		return LanguageList{}, false
	}
	return list, true
}

func writeLanguageCache(list LanguageList) {
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(languageCachePath(), data, 0o644)
}
