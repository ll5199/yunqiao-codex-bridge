package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type proxyImage struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	DownloadURL     string `json:"download_url"`
	FileName        string `json:"file_name"`
	MimeType        string `json:"mime_type"`
	CreatedAt       int64  `json:"created_at"`
	ConversationKey string `json:"conversation_key,omitempty"`
	FilePath        string `json:"-"`
}

type storedImageIndex struct {
	Images []proxyImage `json:"images"`
}

type imageStore struct {
	mu      sync.Mutex
	items   []proxyImage
	seen    map[string]bool
	root    string
	logger  func(string, string)
	baseURL string
}

func imageStorageDirectory() string {
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	if root == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "YunqiaoCodexBridge", "images")
}

func newImageStore(logger func(string, string)) *imageStore {
	return newPersistentImageStore("", logger)
}

func newPersistentImageStore(root string, logger func(string, string)) *imageStore {
	store := &imageStore{
		seen:    make(map[string]bool),
		root:    root,
		logger:  logger,
		baseURL: "http://127.0.0.1:9230",
	}
	store.load()
	return store
}

func (store *imageStore) add(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" || len(source) > maxCaptureBytes {
		return false
	}
	sum := sha256.Sum256([]byte(source))
	id := hex.EncodeToString(sum[:12])

	mimeType, content, ok := decodeImageSource(source)
	fileName := fmt.Sprintf("yunqiao-%s%s", id, imageExtension(mimeType))
	filePath := ""
	publicSource := source
	downloadURL := source
	if ok && store.root != "" {
		if err := os.MkdirAll(store.root, 0700); err == nil {
			filePath = filepath.Join(store.root, fileName)
			if err := os.WriteFile(filePath, content, 0600); err == nil {
				publicSource = store.baseURL + "/yunqiao/image/" + id
				downloadURL = store.baseURL + "/yunqiao/download/" + id
			} else {
				filePath = ""
			}
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.seen[id] {
		return false
	}
	store.seen[id] = true
	store.items = append(store.items, proxyImage{
		ID: id, Source: publicSource, DownloadURL: downloadURL, FileName: fileName,
		MimeType: mimeType, CreatedAt: time.Now().UnixMilli(), FilePath: filePath,
	})
	if len(store.items) > 100 {
		removed := store.items[0]
		delete(store.seen, removed.ID)
		store.items = store.items[len(store.items)-100:]
	}
	store.persistLocked()
	if store.logger != nil {
		store.logger("image.captured", fmt.Sprintf("id=%s bytes=%d persisted=%t", id, len(source), filePath != ""))
	}
	return true
}

func decodeImageSource(source string) (string, []byte, bool) {
	if !strings.HasPrefix(strings.ToLower(source), "data:image/") {
		return "image/png", nil, false
	}
	comma := strings.IndexByte(source, ',')
	if comma < 0 || !strings.Contains(strings.ToLower(source[:comma]), ";base64") {
		return "", nil, false
	}
	mimeType := strings.TrimSpace(strings.Split(source[5:comma], ";")[0])
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.ReplaceAll(source[comma+1:], "\r", ""), "\n", ""))
	return mimeType, content, err == nil
}

func imageExtension(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
			return extensions[0]
		}
		return ".png"
	}
}

func (store *imageStore) list() []proxyImage {
	return store.listForConversation("")
}

func (store *imageStore) listForConversation(key string) []proxyImage {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]proxyImage, 0, len(store.items))
	for _, item := range store.items {
		if key == "" || item.ConversationKey == key {
			result = append(result, item)
		}
	}
	return result
}

func (store *imageStore) associate(ids []string, key string) {
	key = strings.TrimSpace(key)
	if key == "" || len(ids) == 0 {
		return
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[strings.TrimSpace(id)] = true
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	changed := false
	for index := range store.items {
		if wanted[store.items[index].ID] && store.items[index].ConversationKey != key {
			store.items[index].ConversationKey = key
			changed = true
		}
	}
	if changed {
		store.persistLocked()
	}
}

func (store *imageStore) serveImage(writer http.ResponseWriter, request *http.Request, id string, attachment bool) {
	store.mu.Lock()
	var found *proxyImage
	for index := range store.items {
		if store.items[index].ID == id {
			copy := store.items[index]
			found = &copy
			break
		}
	}
	store.mu.Unlock()
	if found == nil || found.FilePath == "" {
		http.NotFound(writer, request)
		return
	}
	if request.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if attachment {
		writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, found.FileName))
	}
	writer.Header().Set("Content-Type", found.MimeType)
	writer.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeFile(writer, request, found.FilePath)
}

func (store *imageStore) load() {
	if store.root == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(store.root, "index.json"))
	if err != nil {
		return
	}
	var index storedImageIndex
	if json.Unmarshal(data, &index) != nil {
		return
	}
	for _, item := range index.Images {
		item.FilePath = filepath.Join(store.root, item.FileName)
		if item.ID == "" || item.FileName == "" {
			continue
		}
		if _, err := os.Stat(item.FilePath); err != nil {
			continue
		}
		item.Source = store.baseURL + "/yunqiao/image/" + item.ID
		item.DownloadURL = store.baseURL + "/yunqiao/download/" + item.ID
		store.items = append(store.items, item)
		store.seen[item.ID] = true
	}
}

func (store *imageStore) persistLocked() {
	if store.root == "" {
		return
	}
	if os.MkdirAll(store.root, 0700) != nil {
		return
	}
	data, err := json.MarshalIndent(storedImageIndex{Images: store.items}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(store.root, "index.json"), data, 0600)
}

func setLocalCORS(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}
