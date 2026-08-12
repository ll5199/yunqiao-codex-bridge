package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const advertisementConfigURL = "https://index.velyn65.com/download/ad.json"
const consultationButtonText = "点我咨询购买"

type advertisementConfig struct {
	Enabled         bool   `json:"enabled"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	ButtonText      string `json:"button_text"`
	URL             string `json:"url"`
	ImageURL        string `json:"image_url,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
}

func defaultAdvertisementConfig() advertisementConfig {
	return advertisementConfig{
		Enabled:     true,
		Title:       "云桥 API 服务",
		Description: "多模型统一接入，套餐与使用说明",
		ButtonText:  consultationButtonText,
		URL:         "https://api.velyn65.com",
	}
}

func (config advertisementConfig) labelText() string {
	return "  " + config.Title + "  ·  " + config.Description
}

func parseAdvertisementConfig(body []byte) (advertisementConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var config advertisementConfig
	if err := decoder.Decode(&config); err != nil {
		return advertisementConfig{}, fmt.Errorf("广告配置不是有效 JSON：%w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return advertisementConfig{}, err
	}
	config.Title = strings.TrimSpace(config.Title)
	config.Description = strings.TrimSpace(config.Description)
	config.ButtonText = strings.TrimSpace(config.ButtonText)
	config.URL = strings.TrimSpace(config.URL)
	config.ImageURL = strings.TrimSpace(config.ImageURL)
	config.BackgroundColor = strings.TrimSpace(config.BackgroundColor)
	if !config.Enabled {
		return config, nil
	}
	if config.Title == "" || config.Description == "" || config.ButtonText == "" {
		return advertisementConfig{}, errors.New("启用广告时，title、description 和 button_text 不能为空")
	}
	if len([]rune(config.Title)) > 32 || len([]rune(config.Description)) > 80 || len([]rune(config.ButtonText)) > 12 {
		return advertisementConfig{}, errors.New("广告文字过长")
	}
	parsedURL, err := url.Parse(config.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return advertisementConfig{}, errors.New("广告 url 必须是有效的 HTTPS 地址")
	}
	if config.ImageURL != "" {
		parsedImageURL, err := url.Parse(config.ImageURL)
		if err != nil || parsedImageURL.Scheme != "https" || parsedImageURL.Host == "" || parsedImageURL.User != nil {
			return advertisementConfig{}, errors.New("广告 image_url 必须是有效的 HTTPS 地址")
		}
	}
	if config.BackgroundColor != "" && !validHexColor(config.BackgroundColor) {
		return advertisementConfig{}, errors.New("广告 background_color 必须是 #RRGGBB 格式")
	}
	return config, nil
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("广告配置只能包含一个 JSON 对象")
		}
		return fmt.Errorf("广告配置末尾存在无效内容：%w", err)
	}
	return nil
}

func fetchAdvertisementConfig(client *http.Client, endpoint string) (advertisementConfig, []byte, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return advertisementConfig{}, nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	response, err := client.Do(request)
	if err != nil {
		return advertisementConfig{}, nil, fmt.Errorf("读取远程广告配置失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return advertisementConfig{}, nil, fmt.Errorf("远程广告配置返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return advertisementConfig{}, nil, fmt.Errorf("读取远程广告配置失败：%w", err)
	}
	config, err := parseAdvertisementConfig(body)
	if err != nil {
		return advertisementConfig{}, nil, err
	}
	return config, body, nil
}

func loadAdvertisementCache(path string) (advertisementConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return advertisementConfig{}, err
	}
	return parseAdvertisementConfig(body)
}

func saveAdvertisementCache(path string, config advertisementConfig) error {
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "ad-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	// Windows does not consistently replace an existing destination with
	// os.Rename, so remove only this validated cache file before publishing.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func newAdvertisementHTTPClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}
