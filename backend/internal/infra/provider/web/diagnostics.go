package web

import (
	"encoding/json"
	"regexp"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

// Diagnostic excerpts of upstream Imagine payloads. They contain product
// availability and error text, never credentials; the redaction is a guard
// against token- or cookie-shaped values appearing in future payload shapes.
var diagnosticSecretPattern = regexp.MustCompile(`(?i)("?[a-z_]*(token|cookie|authorization|sso|session|secret|jwt)[a-z_]*"?\s*[:=]\s*)("[^"]*"|[^,}\s]+)|eyJ[A-Za-z0-9_-]{10,}(\.[A-Za-z0-9_-]+)*|[A-Za-z0-9_-]{40,}`)

const diagnosticExcerptLimit = 512

func diagnosticExcerpt(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	text := diagnosticSecretPattern.ReplaceAllStringFunc(string(data), func(match string) string {
		if sub := diagnosticSecretPattern.FindStringSubmatch(match); sub != nil && sub[1] != "" {
			return sub[1] + `"[REDACTED]"`
		}
		return "[REDACTED]"
	})
	if len(text) > diagnosticExcerptLimit {
		text = text[:diagnosticExcerptLimit] + "…"
	}
	return text
}

// zeroImagineProducts returns the raw upstream product objects whose quota
// decoded to zero remaining, keyed by upstream field name.
func zeroImagineProducts(body []byte, windows []account.QuotaWindow) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil
	}
	fieldByMode := map[string]string{
		account.QuotaModeWebImagePro: "imagePro", account.QuotaModeWebImageEdit: "imageEdit",
		account.QuotaModeWebVideo: "video", account.QuotaModeWebVideo720p: "video720p",
	}
	result := map[string]json.RawMessage{}
	for _, window := range windows {
		if field, ok := fieldByMode[window.Mode]; ok && window.Remaining == 0 && fields[field] != nil {
			result[field] = fields[field]
		}
	}
	return result
}
