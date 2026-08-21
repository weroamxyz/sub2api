package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultGrokMediaObjectStorageMaxDownloadBytes int64 = 512 << 20
	defaultGrokMediaObjectStorageCacheTTL               = 30 * 24 * time.Hour
)

var errGrokMediaObjectStorageTooLarge = errors.New("grok media object exceeds max download bytes")

type grokMediaObjectURLCache interface {
	GetGrokMediaObjectURL(ctx context.Context, requestID string) (string, error)
	SetGrokMediaObjectURL(ctx context.Context, requestID, objectURL string, ttl time.Duration) error
}

func (s *OpenAIGatewayService) grokMediaObjectStorageVideoURL(
	ctx context.Context,
	account *Account,
	requestID string,
	statusBody []byte,
) (string, error) {
	if s == nil || s.cfg == nil || s.grokMediaStorageFactory == nil || account == nil {
		return "", nil
	}
	if !IsGrokVideoStatusBillable(statusBody) {
		return "", nil
	}
	contentURL, err := grokMediaSignedVideoContentURL(statusBody, requestID)
	if err != nil || strings.TrimSpace(contentURL) == "" {
		return "", err
	}
	cfg := s.cfg.Gateway.GrokMedia.ObjectStorage
	if !cfg.Enabled {
		return "", nil
	}
	if !cfg.IsConfigured() {
		return "", fmt.Errorf("grok media object storage is enabled but not fully configured: %s", strings.Join(cfg.MissingCredentialKeys(), ", "))
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", nil
	}

	if cachedURL := s.cachedGrokMediaObjectURL(ctx, requestID); cachedURL != "" {
		return cachedURL, nil
	}

	value, err, _ := s.grokMediaObjectStorageSF.Do(requestID, func() (any, error) {
		if cachedURL := s.cachedGrokMediaObjectURL(ctx, requestID); cachedURL != "" {
			return cachedURL, nil
		}
		return s.uploadGrokMediaVideoObject(ctx, account, requestID, contentURL, cfg)
	})
	if err != nil {
		return "", err
	}
	objectURL, _ := value.(string)
	return objectURL, nil
}

func (s *OpenAIGatewayService) grokMediaObjectStorageImageResponse(
	ctx context.Context,
	responseID string,
	respBody []byte,
) ([]byte, error) {
	if s == nil || s.cfg == nil || s.grokMediaStorageFactory == nil || len(respBody) == 0 {
		return respBody, nil
	}
	cfg := s.cfg.Gateway.GrokMedia.ObjectStorage
	if !cfg.Enabled {
		return respBody, nil
	}
	if !cfg.IsConfigured() {
		return nil, fmt.Errorf("grok media object storage is enabled but not fully configured: %s", strings.Join(cfg.MissingCredentialKeys(), ", "))
	}
	storageCfg := cfg.ToImageStorageConfig()
	storage, err := s.grokMediaStorageFactory(ctx, &storageCfg)
	if err != nil {
		return nil, fmt.Errorf("build grok media object storage: %w", err)
	}
	if storage == nil {
		return nil, fmt.Errorf("grok media object storage is unavailable")
	}
	taskID := grokMediaObjectStorageSafeName(responseID)
	if taskID == "" {
		sum := sha256.Sum256(respBody)
		taskID = hex.EncodeToString(sum[:])
	}
	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	uploader := NewImageResultUploader(
		storage,
		prefix+"images/",
		cfg.MaxDownloadByte,
		nil,
	)
	return uploader.Rewrite(ctx, taskID, respBody)
}

func (s *OpenAIGatewayService) cachedGrokMediaObjectURL(ctx context.Context, requestID string) string {
	cache, ok := s.cache.(grokMediaObjectURLCache)
	if !ok || cache == nil {
		return ""
	}
	objectURL, err := cache.GetGrokMediaObjectURL(ctx, requestID)
	if err != nil {
		slog.Warn("grok_media.object_storage_cache_get_failed", "request_id", requestID, "error", err)
		return ""
	}
	return strings.TrimSpace(objectURL)
}

func (s *OpenAIGatewayService) cacheGrokMediaObjectURL(ctx context.Context, requestID, objectURL string, cfg config.GatewayGrokMediaObjectStorageConfig) {
	cache, ok := s.cache.(grokMediaObjectURLCache)
	if !ok || cache == nil || strings.TrimSpace(objectURL) == "" {
		return
	}
	ttl := defaultGrokMediaObjectStorageCacheTTL
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		ttl = time.Duration(cfg.PresignExpiry) * time.Hour
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		if ttl > 5*time.Minute {
			ttl -= 5 * time.Minute
		}
	}
	if err := cache.SetGrokMediaObjectURL(ctx, requestID, objectURL, ttl); err != nil {
		slog.Warn("grok_media.object_storage_cache_set_failed", "request_id", requestID, "error", err)
	}
}

func (s *OpenAIGatewayService) uploadGrokMediaVideoObject(
	ctx context.Context,
	account *Account,
	requestID string,
	contentURL string,
	cfg config.GatewayGrokMediaObjectStorageConfig,
) (string, error) {
	storageCfg := cfg.ToImageStorageConfig()
	storage, err := s.grokMediaStorageFactory(ctx, &storageCfg)
	if err != nil {
		return "", fmt.Errorf("build grok media object storage: %w", err)
	}
	if storage == nil {
		return "", fmt.Errorf("grok media object storage is unavailable")
	}

	req, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(ctx),
		http.MethodGet,
		contentURL,
		nil,
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "*/*")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", fmt.Errorf("download grok video content: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download grok video content: upstream status %d", resp.StatusCode)
	}

	maxBytes := cfg.MaxDownloadByte
	if maxBytes <= 0 {
		maxBytes = defaultGrokMediaObjectStorageMaxDownloadBytes
	}
	contentLength := resp.ContentLength
	if contentLength < 0 {
		if rawLength := strings.TrimSpace(resp.Header.Get("Content-Length")); rawLength == "" {
			contentLength = -1
		}
	}
	if contentLength > maxBytes {
		return "", fmt.Errorf("%w: content length %d > %d", errGrokMediaObjectStorageTooLarge, contentLength, maxBytes)
	}

	contentType := grokMediaObjectStorageContentType(resp.Header.Get("Content-Type"))
	key := grokMediaObjectStorageVideoKey(cfg.Prefix, requestID, contentType)
	var objectURL string
	if streaming, ok := storage.(StreamingImageStorage); ok {
		limited := &grokMediaObjectStorageLimitReader{r: resp.Body, max: maxBytes}
		objectURL, err = streaming.SaveStream(ctx, key, contentType, limited, contentLength)
	} else {
		var data []byte
		data, err = readGrokMediaObjectStorageBytes(resp.Body, maxBytes)
		if err == nil {
			objectURL, err = storage.Save(ctx, key, contentType, data)
		}
	}
	if err != nil {
		return "", fmt.Errorf("upload grok video to object storage: %w", err)
	}
	s.cacheGrokMediaObjectURL(ctx, requestID, objectURL, cfg)
	return objectURL, nil
}

func grokMediaObjectStorageContentType(raw string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err == nil && strings.TrimSpace(mediaType) != "" {
		return strings.TrimSpace(mediaType)
	}
	return "video/mp4"
}

func grokMediaObjectStorageVideoKey(prefix, requestID, contentType string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	name := grokMediaObjectStorageSafeName(requestID)
	ext := grokMediaObjectStorageExtension(contentType)
	return prefix + "videos/" + name + ext
}

func grokMediaObjectStorageSafeName(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	var b strings.Builder
	for _, r := range requestID {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			_ = b.WriteByte(byte(r))
		default:
			_ = b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), ".-_")
	if name == "" || len(name) > 160 {
		sum := sha256.Sum256([]byte(requestID))
		name = hex.EncodeToString(sum[:])
	}
	return name
}

func grokMediaObjectStorageExtension(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" || strings.EqualFold(contentType, "video/mp4") {
		return ".mp4"
	}
	extensions, err := mime.ExtensionsByType(contentType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}

func readGrokMediaObjectStorageBytes(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultGrokMediaObjectStorageMaxDownloadBytes
	}
	var buf bytes.Buffer
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	if _, err := buf.ReadFrom(limited); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > maxBytes {
		return nil, errGrokMediaObjectStorageTooLarge
	}
	return buf.Bytes(), nil
}

type grokMediaObjectStorageLimitReader struct {
	r    io.Reader
	max  int64
	read int64
}

func (r *grokMediaObjectStorageLimitReader) Read(p []byte) (int, error) {
	if r.max <= 0 {
		r.max = defaultGrokMediaObjectStorageMaxDownloadBytes
	}
	if r.read >= r.max {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n > 0 {
			return 0, errGrokMediaObjectStorageTooLarge
		}
		return 0, err
	}
	remaining := r.max - r.read
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	return n, err
}
