package keymanager

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
)

type Service struct {
	mu     sync.Mutex
	keys   map[string]*KeyState
	logger *slog.Logger
}

type KeyState struct {
	mu      sync.Mutex
	inUse   bool
	revoked bool
}

type KeyHandle struct {
	service  *Service
	apiKeyID string

	mu       sync.Mutex
	released bool
}

func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		keys:   make(map[string]*KeyState),
		logger: logger,
	}
}

func (s *Service) AcquireKey(ctx context.Context, apiKeyID string, requestID string) (*KeyHandle, error) {
	if s == nil {
		return nil, internalerrors.New(internalerrors.ErrInternalError, "keymanager service is nil", nil)
	}
	apiKeyID = strings.TrimSpace(apiKeyID)
	if apiKeyID == "" {
		return nil, internalerrors.New(internalerrors.ErrAuthFailed, "api_key_id is required", nil)
	}
	requestID = strings.TrimSpace(requestID)

	s.mu.Lock()
	st := s.keys[apiKeyID]
	if st == nil {
		st = &KeyState{}
		s.keys[apiKeyID] = st
	}
	st.mu.Lock()
	if st.revoked {
		st.mu.Unlock()
		s.mu.Unlock()
		return nil, internalerrors.New(internalerrors.ErrKeyRevoked, "api key is revoked", nil)
	}
	if st.inUse {
		st.mu.Unlock()
		s.mu.Unlock()
		return nil, internalerrors.New(internalerrors.ErrRateLimited, "api key is already in use", nil)
	}
	st.inUse = true
	st.mu.Unlock()
	s.mu.Unlock()

	if s.logger != nil {
		attrs := []slog.Attr{slog.String("api_key_id", apiKeyID)}
		if requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
		s.logger.DebugContext(ctx, "key acquired", attrsToAny(attrs)...)
	}

	return &KeyHandle{service: s, apiKeyID: apiKeyID}, nil
}

func (h *KeyHandle) Release() {
	if h == nil {
		return
	}

	h.mu.Lock()
	if h.released {
		h.mu.Unlock()
		return
	}
	h.released = true
	s := h.service
	apiKeyID := strings.TrimSpace(h.apiKeyID)
	h.mu.Unlock()

	if s == nil || apiKeyID == "" {
		return
	}

	s.mu.Lock()
	st := s.keys[apiKeyID]
	if st == nil {
		s.mu.Unlock()
		return
	}
	st.mu.Lock()
	st.inUse = false
	st.mu.Unlock()
	s.mu.Unlock()
}

func (s *Service) RevokeKey(apiKeyID string) {
	if s == nil {
		return
	}
	apiKeyID = strings.TrimSpace(apiKeyID)
	if apiKeyID == "" {
		return
	}

	s.mu.Lock()
	st := s.keys[apiKeyID]
	if st == nil {
		st = &KeyState{}
		s.keys[apiKeyID] = st
	}
	st.mu.Lock()
	st.revoked = true
	st.mu.Unlock()
	s.mu.Unlock()
}

func (s *Service) CleanupKey(apiKeyID string) {
	if s == nil {
		return
	}
	apiKeyID = strings.TrimSpace(apiKeyID)
	if apiKeyID == "" {
		return
	}

	s.mu.Lock()
	st := s.keys[apiKeyID]
	if st == nil {
		s.mu.Unlock()
		return
	}
	st.mu.Lock()
	canDelete := !st.inUse && !st.revoked
	st.mu.Unlock()
	if canDelete {
		delete(s.keys, apiKeyID)
	}
	s.mu.Unlock()
}

func attrsToAny(attrs []slog.Attr) []any {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]any, len(attrs))
	for i, a := range attrs {
		out[i] = a
	}
	return out
}
