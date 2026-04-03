package githuboauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/browser"

	"github.com/availhealth/archon/internal/config"
)

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Interval         int    `json:"interval"`
}

type BrowserLoginResult struct {
	Token           Token
	VerificationURI string
	UserCode        string
	OpenedBrowser   bool
}

type DeviceFlow struct {
	clientID   string
	scopes     []string
	httpClient *http.Client
}

func NewDeviceFlow(cfg config.Config) *DeviceFlow {
	return &DeviceFlow{
		clientID: config.EffectiveGitHubOAuthBrowserClientID(cfg.GitHub),
		scopes:   append([]string(nil), cfg.GitHub.OAuthBrowser.Scopes...),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (f *DeviceFlow) Start(ctx context.Context) (DeviceCodeResponse, bool, error) {
	device, err := f.requestDeviceCode(ctx)
	if err != nil {
		return DeviceCodeResponse{}, false, err
	}
	opened := browser.OpenURL(device.VerificationURI) == nil
	return device, opened, nil
}

func (f *DeviceFlow) WaitForToken(ctx context.Context, device DeviceCodeResponse) (Token, error) {
	return f.pollForToken(ctx, device)
}

func (f *DeviceFlow) Login(ctx context.Context) (BrowserLoginResult, error) {
	device, opened, err := f.Start(ctx)
	if err != nil {
		return BrowserLoginResult{}, err
	}
	token, err := f.pollForToken(ctx, device)
	if err != nil {
		return BrowserLoginResult{}, err
	}
	return BrowserLoginResult{Token: token, VerificationURI: device.VerificationURI, UserCode: device.UserCode, OpenedBrowser: opened}, nil
}

func (f *DeviceFlow) requestDeviceCode(ctx context.Context) (DeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", f.clientID)
	if len(f.scopes) > 0 {
		form.Set("scope", strings.Join(f.scopes, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/device/code", strings.NewReader(form.Encode()))
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("build github device code request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("request github device code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return DeviceCodeResponse{}, fmt.Errorf("request github device code failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var device DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("decode github device code response: %w", err)
	}
	if device.Interval <= 0 {
		device.Interval = 5
	}
	return device, nil
}

func (f *DeviceFlow) pollForToken(ctx context.Context, device DeviceCodeResponse) (Token, error) {
	interval := time.Duration(device.Interval) * time.Second
	expiresAt := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	for {
		if time.Now().After(expiresAt) {
			return Token{}, fmt.Errorf("github device flow expired before authorization completed")
		}
		select {
		case <-ctx.Done():
			return Token{}, ctx.Err()
		case <-time.After(interval):
		}

		payload, err := f.requestAccessToken(ctx, device.DeviceCode)
		if err != nil {
			return Token{}, err
		}
		switch payload.Error {
		case "":
			return Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, Scope: payload.Scope, CreatedAt: time.Now().UTC()}, nil
		case "authorization_pending":
			continue
		case "slow_down":
			if payload.Interval > 0 {
				interval = time.Duration(payload.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
			continue
		case "expired_token":
			return Token{}, fmt.Errorf("github device flow expired")
		default:
			if strings.TrimSpace(payload.ErrorDescription) != "" {
				return Token{}, fmt.Errorf("github device flow failed: %s", payload.ErrorDescription)
			}
			return Token{}, fmt.Errorf("github device flow failed: %s", payload.Error)
		}
	}
}

func (f *DeviceFlow) requestAccessToken(ctx context.Context, deviceCode string) (accessTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", f.clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return accessTokenResponse{}, fmt.Errorf("build github access token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return accessTokenResponse{}, fmt.Errorf("request github access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return accessTokenResponse{}, fmt.Errorf("request github access token failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload accessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return accessTokenResponse{}, fmt.Errorf("decode github access token response: %w", err)
	}
	return payload, nil
}
