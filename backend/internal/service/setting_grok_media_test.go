//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGetGrokMediaSignedContentURLSettingsFallsBackToConfig(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			GrokMedia: config.GatewayGrokMediaConfig{
				SignedContentURL: config.GatewayGrokMediaSignedContentURLConfig{
					Enabled:       true,
					TTLSeconds:    1800,
					PublicBaseURL: " https://api.example.com/ ",
				},
			},
		},
	}
	svc := NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{}}, cfg)

	settings, err := svc.GetGrokMediaSignedContentURLSettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 1800, settings.TTLSeconds)
	require.Equal(t, "https://api.example.com", settings.PublicBaseURL)
}

func TestSetGrokMediaSignedContentURLSettingsValidatesAndPersists(t *testing.T) {
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "jwt-secret"}}
	svc := NewSettingService(repo, cfg)

	err := svc.SetGrokMediaSignedContentURLSettings(context.Background(), &GrokMediaSignedContentURLSettings{
		Enabled:       true,
		TTLSeconds:    600,
		PublicBaseURL: " https://cdn.example.com/api/ ",
	})

	require.NoError(t, err)
	require.JSONEq(t,
		`{"enabled":true,"ttl_seconds":600,"public_base_url":"https://cdn.example.com/api"}`,
		repo.values[SettingKeyGrokMediaSignedContentURLSettings],
	)
}

func TestSetGrokMediaSignedContentURLSettingsRejectsUnsafeValues(t *testing.T) {
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.SetGrokMediaSignedContentURLSettings(context.Background(), &GrokMediaSignedContentURLSettings{
		Enabled:       true,
		TTLSeconds:    600,
		PublicBaseURL: "https://api.example.com",
	})
	require.ErrorContains(t, err, "secret or jwt.secret is required")

	err = svc.SetGrokMediaSignedContentURLSettings(context.Background(), &GrokMediaSignedContentURLSettings{
		Enabled:       false,
		TTLSeconds:    600,
		PublicBaseURL: "https://user:pass@api.example.com?token=1",
	})
	require.ErrorContains(t, err, "must not include userinfo or query")
}
