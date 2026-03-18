package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type turnstileResponse struct {
	Success bool     `json:"success"`
	Codes   []string `json:"error-codes"`
}

// verifyTurnstile calls the Cloudflare Turnstile siteverify endpoint and
// returns true if the token is valid for the given secret and remote IP.
func verifyTurnstile(ctx context.Context, secret, token, remoteIP string) (bool, error) {
	if token == "" {
		return false, fmt.Errorf("missing turnstile token")
	}

	body := url.Values{
		"secret":   {secret},
		"response": {token},
		"remoteip": {remoteIP},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result turnstileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Success, nil
}
