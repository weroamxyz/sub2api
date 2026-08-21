package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const defaultGrokMediaSignedContentURLTTLSeconds = 3600

// GrokMediaSignedContentURLSettings stores the admin-managed public URL
// behavior for Grok video content proxy links. The signing secret intentionally
// stays in deployment config/JWT config and is not persisted through admin UI.
type GrokMediaSignedContentURLSettings struct {
	Enabled       bool   `json:"enabled"`
	TTLSeconds    int    `json:"ttl_seconds"`
	PublicBaseURL string `json:"public_base_url"`
}

func DefaultGrokMediaSignedContentURLSettingsFromConfig(cfg *config.Config) *GrokMediaSignedContentURLSettings {
	settings := &GrokMediaSignedContentURLSettings{
		TTLSeconds: defaultGrokMediaSignedContentURLTTLSeconds,
	}
	if cfg == nil {
		return settings
	}
	settings.Enabled = cfg.Gateway.GrokMedia.SignedContentURL.Enabled
	if cfg.Gateway.GrokMedia.SignedContentURL.TTLSeconds > 0 {
		settings.TTLSeconds = cfg.Gateway.GrokMedia.SignedContentURL.TTLSeconds
	}
	settings.PublicBaseURL = normalizeGrokMediaSignedContentPublicBaseURL(
		cfg.Gateway.GrokMedia.SignedContentURL.PublicBaseURL,
	)
	return settings
}

// GetGrokMediaSignedContentURLSettings returns the effective admin setting. A
// missing DB row falls back to config/env defaults for upgrade compatibility.
func (s *SettingService) GetGrokMediaSignedContentURLSettings(ctx context.Context) (*GrokMediaSignedContentURLSettings, error) {
	defaults := DefaultGrokMediaSignedContentURLSettingsFromConfig(nil)
	if s != nil {
		defaults = DefaultGrokMediaSignedContentURLSettingsFromConfig(s.cfg)
	}
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}

	value, err := s.settingRepo.GetValue(ctx, SettingKeyGrokMediaSignedContentURLSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
		}
		return nil, fmt.Errorf("get grok media signed content URL settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return defaults, nil
	}

	var settings GrokMediaSignedContentURLSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return defaults, nil
	}
	normalizeGrokMediaSignedContentURLSettings(&settings, defaults)
	return &settings, nil
}

// SetGrokMediaSignedContentURLSettings validates and persists the admin setting.
func (s *SettingService) SetGrokMediaSignedContentURLSettings(ctx context.Context, settings *GrokMediaSignedContentURLSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	defaults := DefaultGrokMediaSignedContentURLSettingsFromConfig(nil)
	if s != nil {
		defaults = DefaultGrokMediaSignedContentURLSettingsFromConfig(s.cfg)
	}
	normalizeGrokMediaSignedContentURLSettings(settings, defaults)
	if settings.Enabled {
		if settings.TTLSeconds <= 0 {
			return fmt.Errorf("ttl_seconds must be positive")
		}
		if !grokMediaSignedContentSecretConfigured(s.config()) {
			return fmt.Errorf("gateway.grok_media.signed_content_url.secret or jwt.secret is required when enabled")
		}
	}
	if err := validateGrokMediaSignedContentPublicBaseURL(settings.PublicBaseURL); err != nil {
		return err
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal grok media signed content URL settings: %w", err)
	}
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting repository is unavailable")
	}
	return s.settingRepo.Set(ctx, SettingKeyGrokMediaSignedContentURLSettings, string(data))
}

func (s *SettingService) config() *config.Config {
	if s == nil {
		return nil
	}
	return s.cfg
}

func normalizeGrokMediaSignedContentURLSettings(settings, defaults *GrokMediaSignedContentURLSettings) {
	if settings == nil {
		return
	}
	if defaults == nil {
		defaults = &GrokMediaSignedContentURLSettings{TTLSeconds: defaultGrokMediaSignedContentURLTTLSeconds}
	}
	if settings.TTLSeconds <= 0 {
		settings.TTLSeconds = defaults.TTLSeconds
	}
	if settings.TTLSeconds <= 0 {
		settings.TTLSeconds = defaultGrokMediaSignedContentURLTTLSeconds
	}
	settings.PublicBaseURL = normalizeGrokMediaSignedContentPublicBaseURL(settings.PublicBaseURL)
}

func normalizeGrokMediaSignedContentPublicBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func validateGrokMediaSignedContentPublicBaseURL(raw string) error {
	raw = normalizeGrokMediaSignedContentPublicBaseURL(raw)
	if raw == "" {
		return nil
	}
	if err := config.ValidateAbsoluteHTTPURL(raw); err != nil {
		return fmt.Errorf("public_base_url must be an absolute http(s) URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("public_base_url must be an absolute http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" {
		return fmt.Errorf("public_base_url must not include userinfo or query")
	}
	return nil
}

func grokMediaSignedContentSecretConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Gateway.GrokMedia.SignedContentURL.Secret) != "" ||
		strings.TrimSpace(cfg.JWT.Secret) != ""
}
