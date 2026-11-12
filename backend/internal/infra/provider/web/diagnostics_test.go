package web

import (
	"strings"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestDiagnosticExcerptRedactsSecrets(t *testing.T) {
	got := diagnosticExcerpt(map[string]any{
		"type": "error", "message": "rate limited",
		"sso_token": "abc", "cookie": "x=y", "jwt": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig",
		"blob": strings.Repeat("a", 48),
	})
	for _, leaked := range []string{"abc", "x=y", "eyJhbGci", strings.Repeat("a", 48)} {
		if strings.Contains(got, leaked) {
			t.Fatalf("excerpt leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "rate limited") {
		t.Fatalf("excerpt dropped the error text: %s", got)
	}
}

func TestZeroImagineProductsKeepsOnlyZeroWindows(t *testing.T) {
	body := []byte(`{"image":{"available":true},"imagePro":{"available":false,"remainingQueries":0,"windowSizeSeconds":86400},"imageEdit":null,"video":null,"video720p":{"available":true,"remainingQueries":3,"windowSizeSeconds":86400}}`)
	windows := []account.QuotaWindow{{Mode: account.QuotaModeWebImagePro, Remaining: 0}, {Mode: account.QuotaModeWebVideo720p, Remaining: 3}}
	got := zeroImagineProducts(body, windows)
	if len(got) != 1 || !strings.Contains(string(got["imagePro"]), `"available":false`) {
		t.Fatalf("zero products = %v", got)
	}
}
