package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseAdvertisementConfig(t *testing.T) {
	body := []byte(`{"enabled":true,"title":"新套餐","description":"今日开放","button_text":"立即查看","url":"https://api.velyn65.com/pricing"}`)
	config, err := parseAdvertisementConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if config.Title != "新套餐" || config.URL != "https://api.velyn65.com/pricing" {
		t.Fatalf("unexpected advertisement: %#v", config)
	}
}

func TestParseAdvertisementConfigAllowsDisabledAdvertisement(t *testing.T) {
	config, err := parseAdvertisementConfig([]byte(`{"enabled":false,"title":"","description":"","button_text":"","url":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled {
		t.Fatal("advertisement should be disabled")
	}
}

func TestParseAdvertisementConfigRejectsUnsafeURL(t *testing.T) {
	body := []byte(`{"enabled":true,"title":"广告","description":"说明","button_text":"详情","url":"javascript:alert(1)"}`)
	if _, err := parseAdvertisementConfig(body); err == nil {
		t.Fatal("unsafe URL was accepted")
	}
}

func TestFetchAdvertisementConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"enabled":true,"title":"远程广告","description":"远程说明","button_text":"查看","url":"https://api.velyn65.com"}`))
	}))
	defer server.Close()
	config, _, err := fetchAdvertisementConfig(&http.Client{Timeout: time.Second}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if config.Title != "远程广告" {
		t.Fatalf("unexpected title: %s", config.Title)
	}
}

func TestAdvertisementCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ad.json")
	want := defaultAdvertisementConfig()
	if err := saveAdvertisementCache(path, want); err != nil {
		t.Fatal(err)
	}
	want.Description = "更新后的缓存内容"
	if err := saveAdvertisementCache(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadAdvertisementCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("cache mismatch: got %#v want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("cache is empty")
	}
}
