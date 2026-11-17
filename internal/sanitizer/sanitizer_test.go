package sanitizer_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/sanitizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeHeaders(t *testing.T) {
	headers := map[string][]string{
		"Authorization":   {"Bearer secret_jwt_token_123"},
		"Cookie":          {"session_id=xyz789; tracking=false"},
		"Set-Cookie":      {"auth_cookie=abc; HttpOnly"},
		"X-Api-Key":       {"super_secret_api_key"},
		"x-auth-token":    {"auth_token_value"},
		"Content-Type":    {"application/json"},
		"Accept":          {"application/json", "text/plain"},
		"User-Agent":      {"DriftMock/1.0"},
	}

	sanitized := sanitizer.SanitizeHeaders(headers, nil)

	assert.Equal(t, []string{"[REDACTED]"}, sanitized["Authorization"])
	assert.Equal(t, []string{"[REDACTED]"}, sanitized["Cookie"])
	assert.Equal(t, []string{"[REDACTED]"}, sanitized["Set-Cookie"])
	assert.Equal(t, []string{"[REDACTED]"}, sanitized["X-Api-Key"])
	assert.Equal(t, []string{"[REDACTED]"}, sanitized["x-auth-token"])

	// Non-sensitive headers must remain untouched
	assert.Equal(t, []string{"application/json"}, sanitized["Content-Type"])
	assert.Equal(t, []string{"application/json", "text/plain"}, sanitized["Accept"])
	assert.Equal(t, []string{"DriftMock/1.0"}, sanitized["User-Agent"])
}

func TestSanitizeHTTPHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Basic dXNlcjpwYXNz")
	h.Set("X-Custom-Safe", "public_val")

	sanitized := sanitizer.SanitizeHTTPHeader(h, nil)
	assert.Equal(t, "[REDACTED]", sanitized.Get("Authorization"))
	assert.Equal(t, "public_val", sanitized.Get("X-Custom-Safe"))
}

func TestMaskJSONBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, output []byte)
	}{
		{
			name:  "empty payload",
			input: "",
			validate: func(t *testing.T, output []byte) {
				assert.Empty(t, output)
			},
		},
		{
			name:  "non-JSON text payload preserved",
			input: "<html><body>Not JSON</body></html>",
			validate: func(t *testing.T, output []byte) {
				assert.Equal(t, "<html><body>Not JSON</body></html>", string(output))
			},
		},
		{
			name: "flat sensitive keys",
			input: `{
				"username": "pandusatrianura",
				"password": "supersecretpassword",
				"api_key": "live_key_999",
				"nik": "3171234567890001",
				"role": "admin"
			}`,
			validate: func(t *testing.T, output []byte) {
				var res map[string]any
				err := json.Unmarshal(output, &res)
				require.NoError(t, err)
				assert.Equal(t, "pandusatrianura", res["username"])
				assert.Equal(t, "[REDACTED]", res["password"])
				assert.Equal(t, "[REDACTED]", res["api_key"])
				assert.Equal(t, "[REDACTED]", res["nik"])
				assert.Equal(t, "admin", res["role"])
			},
		},
		{
			name: "deeply nested sensitive keys and arrays",
			input: `{
				"data": {
					"user": {
						"id": 101,
						"profile": {
							"auth_token": "bearer_abc",
							"credit_card": "4111222233334444",
							"cvv": "123"
						}
					},
					"accounts": [
						{"id": 1, "secret": "s1"},
						{"id": 2, "pin": 9999, "name": "Main Account"}
					]
				}
			}`,
			validate: func(t *testing.T, output []byte) {
				var res map[string]any
				err := json.Unmarshal(output, &res)
				require.NoError(t, err)

				data := res["data"].(map[string]any)
				user := data["user"].(map[string]any)
				profile := user["profile"].(map[string]any)

				assert.Equal(t, "[REDACTED]", profile["auth_token"])
				assert.Equal(t, "[REDACTED]", profile["credit_card"])
				assert.Equal(t, "[REDACTED]", profile["cvv"])

				accounts := data["accounts"].([]any)
				acc1 := accounts[0].(map[string]any)
				acc2 := accounts[1].(map[string]any)

				assert.Equal(t, "[REDACTED]", acc1["secret"])
				assert.Equal(t, "[REDACTED]", acc2["pin"])
				assert.Equal(t, "Main Account", acc2["name"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := sanitizer.MaskJSONBody([]byte(tt.input), nil)
			require.NoError(t, err)
			tt.validate(t, output)
		})
	}
}

func TestPipelineSanitizeStub(t *testing.T) {
	p := sanitizer.NewPipeline()

	stub := &model.Stub{
		Request: model.RequestPattern{
			Method: "POST",
			Path:   "/api/v1/auth/login",
			Headers: map[string][]string{
				"Authorization": {"Basic dXNlcjpwYXNz"},
				"Content-Type":  {"application/json"},
			},
		},
		Response: model.ResponsePattern{
			StatusCode: 200,
			Headers: map[string][]string{
				"Set-Cookie":   {"jwt=abc123token; Secure"},
				"Content-Type": {"application/json"},
			},
			Body: map[string]any{
				"token":        "jwt.token.value",
				"refreshToken": "refresh.token.value",
				"user": map[string]any{
					"name":     "Pandu",
					"password": "hashed_pass",
				},
			},
		},
	}

	p.SanitizeStub(stub)

	// Check request
	assert.Equal(t, []string{"[REDACTED]"}, stub.Request.Headers["Authorization"])
	assert.Equal(t, []string{"application/json"}, stub.Request.Headers["Content-Type"])

	// Check response headers
	assert.Equal(t, []string{"[REDACTED]"}, stub.Response.Headers["Set-Cookie"])
	assert.Equal(t, []string{"application/json"}, stub.Response.Headers["Content-Type"])

	// Check response body
	body := stub.Response.Body.(map[string]any)
	assert.Equal(t, "[REDACTED]", body["token"])
	assert.Equal(t, "[REDACTED]", body["refreshToken"])

	user := body["user"].(map[string]any)
	assert.Equal(t, "Pandu", user["name"])
	assert.Equal(t, "[REDACTED]", user["password"])
}

func TestPipelineSanitizeRawResponseBody(t *testing.T) {
	p := sanitizer.NewPipeline()

	// 1. JSON body
	jsonInput := []byte(`{"access_token": "secret123", "expires_in": 3600}`)
	sanitized, err := p.SanitizeRawResponseBody(jsonInput)
	require.NoError(t, err)

	bodyMap, ok := sanitized.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "[REDACTED]", bodyMap["access_token"])
	assert.Equal(t, float64(3600), bodyMap["expires_in"])

	// 2. Non-JSON body
	plainInput := []byte("plain text response")
	plainSanitized, err := p.SanitizeRawResponseBody(plainInput)
	require.NoError(t, err)
	assert.Equal(t, "plain text response", plainSanitized)

	// 3. Nil / empty
	emptySanitized, err := p.SanitizeRawResponseBody(nil)
	require.NoError(t, err)
	assert.Nil(t, emptySanitized)
}
