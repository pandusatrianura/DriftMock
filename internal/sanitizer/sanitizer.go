package sanitizer

import (
	"encoding/json"

	"github.com/pandusatrianura/driftmock/internal/model"
)

// Pipeline manages sanitization for HTTP requests and responses.
type Pipeline struct {
	sensitiveHeaders map[string]bool
	sensitiveKeys    map[string]bool
}

// NewPipeline creates a new Sanitizer Pipeline with default rules.
func NewPipeline() *Pipeline {
	return &Pipeline{
		sensitiveHeaders: DefaultSensitiveHeaders,
		sensitiveKeys:    DefaultSensitiveKeys,
	}
}

// WithCustomSensitiveHeader adds an extra sensitive header to redact.
func (p *Pipeline) WithCustomSensitiveHeader(header string) *Pipeline {
	p.sensitiveHeaders[header] = true
	return p
}

// WithCustomSensitiveKey adds an extra sensitive JSON key to mask.
func (p *Pipeline) WithCustomSensitiveKey(key string) *Pipeline {
	p.sensitiveKeys[key] = true
	return p
}

// SanitizeRequest sanitizes headers and query parameters in a RequestPattern in place.
func (p *Pipeline) SanitizeRequest(req *model.RequestPattern) {
	if req == nil {
		return
	}
	req.Headers = SanitizeHeaders(req.Headers, p.sensitiveHeaders)
	req.Query = SanitizeQueryParams(req.Query, p.sensitiveKeys)
}

// SanitizeResponse sanitizes headers and body in a ResponsePattern in place.
func (p *Pipeline) SanitizeResponse(resp *model.ResponsePattern) {
	if resp == nil {
		return
	}
	resp.Headers = SanitizeHeaders(resp.Headers, p.sensitiveHeaders)
	if resp.Body != nil {
		resp.Body = MaskValue(resp.Body, p.sensitiveKeys)
	}
}

// SanitizeStub applies request and response sanitization to an entire Stub.
func (p *Pipeline) SanitizeStub(stub *model.Stub) {
	if stub == nil {
		return
	}
	p.SanitizeRequest(&stub.Request)
	p.SanitizeResponse(&stub.Response)
}

// SanitizeRawResponse parses raw JSON body bytes if present, masks sensitive keys,
// and returns the parsed, sanitized body object suitable for model.ResponsePattern.Body.
func (p *Pipeline) SanitizeRawResponseBody(data []byte) (any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		// Non-JSON or binary payload: store as string
		return string(data), nil
	}

	return MaskValue(root, p.sensitiveKeys), nil
}
