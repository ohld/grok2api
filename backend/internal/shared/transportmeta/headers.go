// Package transportmeta defines internal HTTP metadata shared across transport
// middleware and protocol handlers. Provider responses must never be allowed to
// mint these gateway-owned assertions.
package transportmeta

const (
	UpstreamRequestDispositionHeader = "X-Upstream-Request-Disposition"
	UpstreamRequestNotSubmitted      = "not-submitted"
)

// IsConversationInferencePath reports whether the request can submit a
// conversation payload through the shared inference gateway.
func IsConversationInferencePath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/responses"
}
