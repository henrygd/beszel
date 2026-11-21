package hub

import (
	"net/http"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/security"
	"github.com/spf13/cast"
)

// Limits for the lifetime of user-generated API keys.
const (
	apiKeyMinDuration = time.Hour
	apiKeyMaxDuration = 366 * 24 * time.Hour
	// JWTs require an expiration, so keys that "never expire" get one far in the future
	apiKeyNoExpiryDuration = 100 * 365 * 24 * time.Hour
)

// requireSessionToken allows only regular user login sessions, so a static
// API key can't mint new keys (extending its own lifetime indefinitely) or
// trade itself for a refreshable session token via revocation.
//
// Static tokens (from this API or PocketBase impersonation) carry
// refreshable=false; tokens from a normal login carry refreshable=true.
func requireSessionToken(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("The request requires valid record authorization token.", nil)
	}
	if e.Auth.Collection().Name != "users" {
		return e.ForbiddenError("Only users can manage API keys.", nil)
	}
	token := strings.TrimPrefix(e.Request.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return e.ForbiddenError("API keys can only be managed from a logged in session.", nil)
	}
	// verify the token itself rather than trusting e.Auth, which may have been
	// set by AUTO_LOGIN or TRUSTED_AUTH_HEADER independently of the token
	record, err := e.App.FindAuthRecordByToken(token, core.TokenTypeAuth)
	if err != nil || record.Id != e.Auth.Id || record.Collection().Id != e.Auth.Collection().Id {
		return e.ForbiddenError("API keys can only be managed from a logged in session.", nil)
	}
	claims, _ := security.ParseUnverifiedJWT(token)
	if !cast.ToBool(claims[core.TokenClaimRefreshable]) {
		return e.ForbiddenError("API keys can't be used to manage API keys.", nil)
	}
	return e.Next()
}

// createApiKey handles POST /api/beszel/api-keys. It returns a static,
// non-refreshable auth token for the current user with the requested duration
// (in seconds), or effectively no expiration if noExpiry is set. The token has
// the same permissions as the user. The response's expires is empty for keys
// that don't expire.
func createApiKey(e *core.RequestEvent) error {
	var body struct {
		Duration int64 `json:"duration"`
		NoExpiry bool  `json:"noExpiry"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("Invalid request body.", err)
	}
	duration := time.Duration(body.Duration) * time.Second
	if body.NoExpiry {
		duration = apiKeyNoExpiryDuration
	} else if duration < apiKeyMinDuration || duration > apiKeyMaxDuration {
		return e.BadRequestError("Duration must be between one hour and 366 days.", nil)
	}
	token, err := e.Auth.NewStaticAuthToken(duration)
	if err != nil {
		return e.InternalServerError("Failed to generate API key.", err)
	}
	expires := ""
	if !body.NoExpiry {
		expires = time.Now().Add(duration).UTC().Format(time.RFC3339)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"expires": expires,
	})
}

// revokeApiKeys handles POST /api/beszel/api-keys/revoke. It rotates the
// user's token key, which invalidates every token issued to the user (API keys
// and login sessions alike), and returns a fresh session token so the caller
// stays logged in.
func revokeApiKeys(e *core.RequestEvent) error {
	user, err := e.App.FindRecordById(e.Auth.Collection(), e.Auth.Id)
	if err != nil {
		return e.NotFoundError("", err)
	}
	user.RefreshTokenKey()
	if err := e.App.Save(user); err != nil {
		return e.InternalServerError("Failed to revoke API keys.", err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		return e.InternalServerError("Failed to generate auth token.", err)
	}
	return e.JSON(http.StatusOK, map[string]string{"token": token})
}
