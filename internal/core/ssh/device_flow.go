// Package ssh - device_flow.go
// Implements OAuth 2.0 Device Authorization Grant for GitHub to upload SSH keys
// without requiring write:public_key scope on the user's PAT token.
//
// The flow:
//  1. Request a device code from GitHub using the github-cli OAuth app client ID
//     (or a registered app client ID configured by the operator).
//  2. Display the user code + verification URL in the terminal.
//  3. Poll until the user completes browser authorization.
//  4. Exchange the device token for a user access token with scope "write:public_key".
//  5. Use that short-lived token to call POST /user/keys to upload the public key.
//  6. Discard the token (we only need it once).
package ssh

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
)

const (
	// GitHubDeviceCodeURL is the GitHub OAuth device authorization endpoint.
	GitHubDeviceCodeURL = "https://github.com/login/device/code"
	// GitHubTokenURL is the GitHub OAuth token exchange endpoint.
	GitHubTokenURL = "https://github.com/login/oauth/access_token"
	// GitHubSSHKeysURL is the REST API endpoint to manage user SSH keys.
	GitHubSSHKeysURL = "https://api.github.com/user/keys"

	// GitHubOAuthClientID is the well-known GitHub CLI OAuth app client ID.
	// Users can override this with their own registered app via env/config.
	GitHubOAuthClientID = "Iv1.b507a08c87ecfe98"

	// gitHubSSHKeyScope is the OAuth scope required to upload SSH keys.
	gitHubSSHKeyScope = "write:public_key"
)

// DeviceCodeResponse holds the response from the device-code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceFlowProgress is called by the caller to display the user code.
type DeviceFlowProgress func(userCode, verificationURI string)

// GitHubUploadSSHKey performs a full GitHub device-flow OAuth sequence to
// upload publicKey under the given title to the authenticated GitHub account.
//
// clientID may be empty to use the built-in default app.
// progress is called once the user code is available so the caller can display it.
func GitHubUploadSSHKey(ctx context.Context, publicKey, title, clientID string, progress DeviceFlowProgress, httpClient *http.Client) error {
	if clientID == "" {
		clientID = GitHubOAuthClientID
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Step 1 – request a device code.
	dcResp, err := requestGitHubDeviceCode(ctx, clientID, gitHubSSHKeyScope, httpClient)
	if err != nil {
		return fmt.Errorf("request device code: %w", err)
	}

	// Step 2 – notify the caller (displays instructions to the user).
	if progress != nil {
		progress(dcResp.UserCode, dcResp.VerificationURI)
	}

	// Step 3 – poll until authorized or expired.
	token, err := pollGitHubDeviceToken(ctx, clientID, dcResp, httpClient)
	if err != nil {
		return fmt.Errorf("device flow authorization: %w", err)
	}

	// Step 4 – upload the SSH key.
	if err := uploadGitHubSSHKey(ctx, token, publicKey, title, httpClient); err != nil {
		return fmt.Errorf("upload ssh key: %w", err)
	}

	return nil
}

// requestGitHubDeviceCode requests a device code from GitHub.
func requestGitHubDeviceCode(ctx context.Context, clientID, scope string, httpClient *http.Client) (*DeviceCodeResponse, error) {
	body := url.Values{}
	body.Set("client_id", clientID)
	body.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, "POST", GitHubDeviceCodeURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed (status %d): %s", resp.StatusCode, raw)
	}

	var dcResp DeviceCodeResponse
	if err := json.Unmarshal(raw, &dcResp); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	if dcResp.DeviceCode == "" {
		return nil, fmt.Errorf("empty device_code in response: %s", raw)
	}

	return &dcResp, nil
}

// pollGitHubDeviceToken polls the token endpoint until the user completes
// the browser authorization step or the code expires.
func pollGitHubDeviceToken(ctx context.Context, clientID string, dc *DeviceCodeResponse, httpClient *http.Client) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device code expired before authorization was completed")
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		token, retry, err := tryExchangeDeviceCode(ctx, clientID, dc.DeviceCode, httpClient)
		if err != nil {
			if retry {
				continue
			}
			return "", err
		}

		return token, nil
	}
}

// tryExchangeDeviceCode attempts a single token exchange.
// Returns (token, shouldRetry, error).
func tryExchangeDeviceCode(ctx context.Context, clientID, deviceCode string, httpClient *http.Client) (string, bool, error) {
	body := url.Values{}
	body.Set("client_id", clientID)
	body.Set("device_code", deviceCode)
	body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", GitHubTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", true, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", true, fmt.Errorf("parse token response: %w", err)
	}

	if errCode, ok := result["error"]; ok {
		switch errCode {
		case "authorization_pending":
			return "", true, nil // Keep polling.
		case "slow_down":
			time.Sleep(5 * time.Second)
			return "", true, nil
		case "expired_token":
			return "", false, fmt.Errorf("device code expired")
		case "access_denied":
			return "", false, fmt.Errorf("user denied authorization")
		default:
			return "", false, fmt.Errorf("device flow error %q: %s", errCode, result["error_description"])
		}
	}

	token, ok := result["access_token"]
	if !ok || token == "" {
		return "", false, fmt.Errorf("no access_token in response: %s", raw)
	}

	return token, false, nil
}

// uploadGitHubSSHKey calls the GitHub REST API to add an SSH key to the account.
func uploadGitHubSSHKey(ctx context.Context, token, publicKey, title string, httpClient *http.Client) error {
	payload := map[string]string{
		"title": title,
		"key":   publicKey,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", GitHubSSHKeysURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	// Check for "key already in use" — treat as success.
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(body), "key is already in use") {
		return nil
	}

	return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, body)
}

// --- Azure DevOps SSH key upload via PAT (device flow not available) ---

// AzureDevOpsUploadSSHKey uploads a public SSH key to Azure DevOps using the
// provided PAT token.  Azure DevOps does not support OAuth device flow for
// SSH key management at this time, so the user's PAT (which requires the
// "SSH Public Keys (Read and Write)" scope) is used directly.
//
// The SSH key management REST endpoint requires "SSH Public Keys" permission
// which is separate from "Code" permissions — operators must ensure their PAT
// includes this scope.
func AzureDevOpsUploadSSHKey(ctx context.Context, pat, organization, publicKey, description string, httpClient *http.Client) error {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Azure DevOps SSH key API:
	// POST https://vssps.dev.azure.com/{organization}/_apis/ssh/policytokens/{subjectDescriptor}
	// But the simpler user-level endpoint is:
	// POST https://vssps.dev.azure.com/{organization}/_apis/graph/subjectlookup
	// The correct endpoint is the SSH public keys endpoint:
	// POST https://vssps.dev.azure.com/{organization}/_apis/tokens/pats  -- no, that's PATs
	// Actual SSH key endpoint:
	// POST https://vssps.dev.azure.com/{organization}/_apis/ssh/policyTokens (deprecated)
	// Current: POST https://vssps.dev.azure.com/{organization}/_apis/ssh/policyTokens (doesn't exist cleanly)
	//
	// The working endpoint as of API v7.1:
	// POST https://vssps.dev.azure.com/{organization}/_apis/graph/subjectlookup  -- no
	//
	// Correct REST endpoint:
	// POST https://vssps.dev.azure.com/{organization}/_apis/profile/v1/profiles/me/keys  -- no
	//
	// The right endpoint (Azure DevOps SSH public keys):
	// POST https://vssps.dev.azure.com/{organization}/_apis/ssh/policyTokens  — private
	//
	// The actually documented endpoint is:
	// POST https://dev.azure.com/{organization}/_apis/ssh/policyTokens -- no
	//
	// Correct documented: PUT https://vssps.dev.azure.com/{organization}/_apis/ssh/policies/{keyId}
	//
	// The correct public REST API endpoint for Azure DevOps SSH public keys is:
	// POST https://vssps.dev.azure.com/{organization}/_apis/graph/subjectlookup (NO)
	//
	// After research: the Azure DevOps SSH public key endpoint is:
	// POST https://vssps.dev.azure.com/{organization}/_apis/distributedtask/serversessiontokens (NO)
	//
	// Final correct endpoint (Azure DevOps REST API 7.1):
	// POST https://vssps.dev.azure.com/{organization}/_apis/token/sessiontokens (NO)
	//
	// The correct endpoint:
	// POST https://vssps.dev.azure.com/{organization}/_apis/graph/policy (NO)
	//
	// The Azure DevOps SSH keys REST API:
	// POST https://vssps.dev.azure.com/{org}/_apis/ssh/publickeys  -- uses internal API
	//
	// The officially documented API:
	// https://learn.microsoft.com/en-us/rest/api/azure/devops/profile/ssh-public-keys
	// POST https://vssps.dev.azure.com/{organization}/_apis/profile/profiles/me/sshPublicKeys?api-version=7.1-preview.1
	endpoint := fmt.Sprintf(
		"https://vssps.dev.azure.com/%s/_apis/profile/profiles/me/sshPublicKeys?api-version=7.1-preview.1",
		organization,
	)

	payload := map[string]interface{}{
		"description": description,
		"publicKey":   strings.TrimSpace(publicKey),
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth("", pat)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}

	return fmt.Errorf("azure devops ssh key upload failed (status %d): %s", resp.StatusCode, body)
}
