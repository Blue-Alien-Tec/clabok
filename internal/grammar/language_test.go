package grammar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func languageListServer(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/languages" {
			fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"},`+
				`{"code":"de-DE","longCode":"de-DE","name":"German (Germany)"}]`)
			return
		}
		fmt.Fprint(w, `{"matches":[]}`)
	}))
}

func TestResolve(t *testing.T) {
	settings := model.Settings{Language: "de-DE"}
	if got := Resolve(settings, "fr-FR"); got != "de-DE" {
		t.Fatalf("book language wins: %q", got)
	}
	if got := Resolve(model.Settings{}, "fr-FR"); got != "fr-FR" {
		t.Fatalf("project default: %q", got)
	}
	if got := Resolve(model.Settings{}, ""); got != DefaultLanguage {
		t.Fatalf("built-in default: %q", got)
	}
}

func TestSupportedUsesCacheAndRefreshesOnMiss(t *testing.T) {
	var calls int32
	server := languageListServer(t, &calls)
	defer server.Close()
	client := NewClient(server.URL)
	seedLanguages(t, server.URL, "en-US", "de-DE")

	language, ok, err := client.Supported(context.Background(), "de-de")
	if err != nil || !ok || language.Code != "de-DE" {
		t.Fatalf("cache hit: %+v %v %v", language, ok, err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("a fresh cache must not fetch, got %d requests", got)
	}
	_, ok, err = client.Supported(context.Background(), "fr-FR")
	if err != nil || ok {
		t.Fatalf("unknown language: ok %v err %v", ok, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("a miss must refresh once, got %d requests", got)
	}
}

func TestSupportedUsesAStaleCacheWhenTheServerIsGone(t *testing.T) {
	server := languageListServer(t, nil)
	url := server.URL
	server.Close()
	client := NewClient(url)
	list := LanguageList{
		Endpoint: url, Fetched: time.Now().Add(-2 * LanguageCacheTTL),
		Languages: []Language{{Code: "en-US", Name: "English (US)"}},
	}
	seedCacheFile(t, list)

	language, ok, err := client.Supported(context.Background(), "en-US")
	if err != nil || !ok || language.Code != "en-US" {
		t.Fatalf("stale fallback: %+v %v %v", language, ok, err)
	}
	if _, _, err := client.Supported(context.Background(), "fr-FR"); err == nil {
		t.Fatal("an unknown id against a dead server must report the transport error")
	}
}

func TestCheckSendsLanguageAndDetectsAuto(t *testing.T) {
	var languageValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/languages" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"code":"de-DE","longCode":"de-DE","name":"German"}]`)
			return
		}
		_ = r.ParseForm()
		languageValue = r.Form.Get("language")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[],"language":{"code":"de-DE",`+
			`"detectedLanguage":{"code":"de-DE"}}}`)
	}))
	defer server.Close()
	client := NewClient(server.URL)

	result, err := client.Check(context.Background(), "text", "de-DE")
	if err != nil || result.Language != "de-DE" || languageValue != "de-DE" {
		t.Fatalf("result %+v language %q err %v", result, languageValue, err)
	}
	result, err = client.Check(context.Background(), "text", "auto")
	if err != nil || result.Language != "de-DE" || languageValue != "auto" {
		t.Fatalf("auto result %+v language %q err %v", result, languageValue, err)
	}
}

func TestLanguageSnapshot(t *testing.T) {
	var calls int32
	server := languageListServer(t, &calls)
	defer server.Close()
	client := NewClient(server.URL)

	languages, cached, stale, fetched := languageSnapshot(client, false)
	if cached || stale || fetched == "" || len(languages) != 2 {
		t.Fatalf("first snapshot: %d cached %v stale %v fetched %q", len(languages), cached, stale, fetched)
	}
	atomic.StoreInt32(&calls, 0)
	languages, cached, stale, _ = languageSnapshot(client, false)
	if !cached || stale || len(languages) != 2 || atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("second snapshot: cached %v stale %v calls %d", cached, stale, calls)
	}
	atomic.StoreInt32(&calls, 0)
	if _, _, _, _ = languageSnapshot(client, true); atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("refresh must fetch once, got %d", calls)
	}
}

func seedCacheFile(t *testing.T, list LanguageList) {
	t.Helper()
	writeLanguageCache(list)
}
