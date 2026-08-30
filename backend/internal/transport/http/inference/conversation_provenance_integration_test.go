package inference

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/repository"
	"github.com/chenyme/grok2api/backend/internal/shared/transportmeta"
	transportmiddleware "github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func TestConversationProvenanceFollowsRealGatewaySubmissionBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chat", path: "/v1/chat/completions",
			body: `{"model":"grok-provenance","messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "responses", path: "/v1/responses",
			body: `{"model":"grok-provenance","input":"hello"}`,
		},
	}
	scenarios := []struct {
		name            string
		mode            conversationProvenanceMode
		wantStatus      int
		wantCalls       int64
		wantDisposition string
	}{
		{
			name: "no account", mode: conversationProvenanceNoAccount,
			wantStatus: http.StatusServiceUnavailable, wantDisposition: transportmeta.UpstreamRequestNotSubmitted,
		},
		{
			name: "saturated account", mode: conversationProvenanceSaturated,
			wantStatus: http.StatusServiceUnavailable, wantDisposition: transportmeta.UpstreamRequestNotSubmitted,
		},
		{
			name: "observed 429 then selector exhaustion", mode: conversationProvenanceObserved429,
			wantStatus: http.StatusTooManyRequests, wantCalls: 1,
		},
	}

	for _, endpoint := range endpoints {
		for _, scenario := range scenarios {
			t.Run(endpoint.name+"/"+scenario.name, func(t *testing.T) {
				service, key, adapter := newConversationProvenanceGateway(t, scenario.mode)
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(transportmiddleware.ClientKey, key)
					c.Set(transportmiddleware.RequestIDKey, "req-provenance")
					c.Next()
				})
				NewHandler(service, nil, 1<<20).Register(router.Group("/v1"))

				request := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body))
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)

				if recorder.Code != scenario.wantStatus {
					t.Fatalf("status = %d, want %d; body=%s", recorder.Code, scenario.wantStatus, recorder.Body.String())
				}
				if calls := adapter.calls.Load(); calls != scenario.wantCalls {
					t.Fatalf("ForwardResponse calls = %d, want %d", calls, scenario.wantCalls)
				}
				if disposition := recorder.Header().Get(transportmeta.UpstreamRequestDispositionHeader); disposition != scenario.wantDisposition {
					t.Fatalf("disposition = %q, want %q; body=%s", disposition, scenario.wantDisposition, recorder.Body.String())
				}
			})
		}
	}
}

func TestConversationFirstSubmissionThenSelectorExhaustionReturnsUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		path string
		body string
		call func(*gateway.Service, context.Context, gateway.Input) (*gateway.Result, error)
	}{
		{
			name: "chat", path: "/v1/chat/completions",
			body: `{"model":"grok-provenance","messages":[{"role":"user","content":"hello"}]}`,
			call: func(service *gateway.Service, ctx context.Context, input gateway.Input) (*gateway.Result, error) {
				return service.CreateChatCompletion(ctx, input)
			},
		},
		{
			name: "responses", path: "/v1/responses",
			body: `{"model":"grok-provenance","input":"hello"}`,
			call: func(service *gateway.Service, ctx context.Context, input gateway.Input) (*gateway.Result, error) {
				return service.CreateResponse(ctx, input)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, key, adapter := newConversationProvenanceGateway(t, conversationProvenanceObserved429)
			result, err := test.call(service, context.Background(), gateway.Input{
				RequestID: "req-provenance-direct", ClientKey: key, PublicModel: "grok-provenance",
				Body: []byte(test.body), Method: http.MethodPost, Path: test.path,
			})
			if result != nil {
				t.Fatal("post-submit exhaustion unexpectedly returned a Result")
			}
			var upstreamFailure *gateway.UpstreamFailure
			if !errors.As(err, &upstreamFailure) || upstreamFailure.HTTPStatus != http.StatusTooManyRequests {
				t.Fatalf("error = %T %v, want 429 UpstreamFailure", err, err)
			}
			if calls := adapter.calls.Load(); calls != 1 {
				t.Fatalf("ForwardResponse calls = %d, want 1", calls)
			}

			recorder := httptest.NewRecorder()
			requestContext, _ := gin.CreateTestContext(recorder)
			requestContext.Request = httptest.NewRequest(http.MethodPost, test.path, nil)
			writeGatewayError(requestContext, err)
			if disposition := recorder.Header().Get(transportmeta.UpstreamRequestDispositionHeader); disposition != "" {
				t.Fatalf("UpstreamFailure minted disposition = %q", disposition)
			}
		})
	}
}

type conversationProvenanceMode uint8

const (
	conversationProvenanceNoAccount conversationProvenanceMode = iota
	conversationProvenanceSaturated
	conversationProvenanceObserved429
)

type conversationProvenanceAdapter struct {
	calls atomic.Int64
}

func (a *conversationProvenanceAdapter) Provider() accountdomain.Provider {
	return accountdomain.ProviderBuild
}

func (a *conversationProvenanceAdapter) Definition() provider.Definition {
	return provider.Definition{
		Provider: accountdomain.ProviderBuild,
		Conversation: provider.ConversationSurface{
			Responses: true, ChatCompletions: true,
		},
		Inference: provider.InferencePolicy{Usage: provider.UsageUpstream},
	}
}

func (a *conversationProvenanceAdapter) ForwardResponse(context.Context, provider.ResponseResourceRequest) (*provider.Response, error) {
	a.calls.Add(1)
	return &provider.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":"limited"}`)),
	}, nil
}

func newConversationProvenanceGateway(
	t *testing.T,
	mode conversationProvenanceMode,
) (*gateway.Service, clientkeydomain.Key, *conversationProvenanceAdapter) {
	t.Helper()
	ctx := context.Background()
	database, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "conversation-provenance.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}

	accountRepo := relational.NewAccountRepository(database)
	modelRepo := relational.NewModelRepository(database)
	auditRepo := relational.NewAuditRepository(database)
	responseRepo := relational.NewResponseRepository(database)
	if err := modelRepo.UpsertDiscovered(ctx, accountdomain.ProviderBuild, []string{"grok-provenance"}); err != nil {
		t.Fatal(err)
	}

	limiter := memory.NewConcurrencyLimiter()
	credential, _, createErr := accountRepo.UpsertByIdentity(ctx, accountdomain.Credential{
		Provider: accountdomain.ProviderBuild, Name: "provenance", SourceKey: "provenance",
		EncryptedAccessToken: "encrypted", ExpiresAt: time.Now().Add(time.Hour),
		Enabled: true, AuthStatus: accountdomain.AuthStatusActive, MaxConcurrent: 1,
	})
	if createErr != nil {
		t.Fatal(createErr)
	}
	if err := modelRepo.ReplaceAccountCapabilities(ctx, credential.ID, []string{"grok-provenance"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if mode == conversationProvenanceSaturated {
		release, acquired, acquireErr := limiter.Acquire(ctx, repository.AccountConcurrencyKey(credential.ID), 1)
		if acquireErr != nil || !acquired {
			t.Fatalf("saturate account: acquired=%t err=%v", acquired, acquireErr)
		}
		t.Cleanup(release)
	}

	adapter := &conversationProvenanceAdapter{}
	registry := provider.NewRegistry(adapter)
	sticky := memory.NewStickyStore()
	cipher, err := security.NewCipher("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	accountService := accountapp.NewService(
		accountRepo,
		auditRepo,
		memory.NewDeviceSessionStore(),
		sticky,
		registry,
		cipher,
		nil,
	)
	selector := gateway.NewSelector(accountRepo, limiter, sticky, registry, time.Hour, time.Second, time.Minute)
	service := gateway.NewService(
		modelRepo,
		auditRepo,
		accountService,
		clientkeyapp.NewService(nil, nil, nil, 60, 4, nil),
		registry,
		selector,
		responseRepo,
		2,
	)
	key := clientkeydomain.Key{ID: 1, Name: "provenance", Enabled: true, RPMLimit: 120, MaxConcurrent: 8}
	if mode == conversationProvenanceNoAccount {
		// The route remains publicly resolvable, but this key has no account in
		// scope. The service must return typed SelectionNoAccounts without ever
		// entering the Provider Adapter.
		key.ProviderScope = clientkeydomain.ProviderScopeWeb
	}
	return service, key, adapter
}
