package configsync

import (
	"context"
	"errors"
	"fmt"
	"knot/pkg/config"
)

var (
	ErrRemoteNotFound = errors.New("remote sync archive not found")
	ErrAuthFailed     = errors.New("sync provider authentication failed")
	ErrPermission     = errors.New("sync provider permission denied")
)

type Provider interface {
	Alias() string
	Download(ctx context.Context) ([]byte, error)
	Upload(ctx context.Context, data []byte) error
}

func NewProvider(cfg config.SyncProviderConfig) (Provider, error) {
	switch cfg.Type {
	case config.SyncProviderWebDAV:
		return NewWebDAVProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported sync provider type: %s", cfg.Type)
	}
}
