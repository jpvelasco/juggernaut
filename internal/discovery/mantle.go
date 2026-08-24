package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const maxMantleCatalogBytes = 8 << 20

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type requestSigner func(context.Context, *http.Request) error

var mantleHTTPClient httpDoer = http.DefaultClient

// mantleRequestTimeout bounds each Mantle catalog fetch even when the caller
// passes a context without a deadline (cmd callers pass cmd.Context()).
// Without it, a black-holed connection — VPN drop, DROP-mode firewall,
// stalled TLS handshake — hangs `models refresh` indefinitely. Mirrors
// bedrock connectivity's client timeout for the same class of probe.
// Test-visible so the bound can be tightened without real network waits.
var mantleRequestTimeout = 15 * time.Second

// ListMantleModels queries the account- and region-specific Mantle Models API.
// A bearer token is used when supplied; otherwise the request is SigV4-signed
// with the default AWS credential chain and service name "bedrock-mantle".
func ListMantleModels(ctx context.Context, region, bearerToken string) ([]DiscoveredModel, error) {
	endpoint := fmt.Sprintf("https://bedrock-mantle.%s.api.aws/v1/models", region)

	var sign requestSigner
	if bearerToken == "" {
		cfg, err := loadAWSConfig(ctx, region)
		if err != nil {
			return nil, err
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return nil, fmt.Errorf("retrieving AWS credentials: %w", err)
		}
		sign = func(ctx context.Context, req *http.Request) error {
			emptyHash := sha256.Sum256(nil)
			payloadHash := hex.EncodeToString(emptyHash[:])
			return v4.NewSigner().SignHTTP(ctx, creds, req, payloadHash, "bedrock-mantle", region, time.Now())
		}
	}

	return listMantleModelsWith(ctx, endpoint, bearerToken, mantleHTTPClient, sign)
}

func listMantleModelsWith(
	ctx context.Context,
	endpoint, bearerToken string,
	client httpDoer,
	sign requestSigner,
) ([]DiscoveredModel, error) {
	ctx, cancel := context.WithTimeout(ctx, mantleRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Mantle models request: %w", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	} else {
		if sign == nil {
			return nil, fmt.Errorf("signing Mantle models request: no bearer token or AWS signer available")
		}
		if err := sign(ctx, req); err != nil {
			return nil, fmt.Errorf("signing Mantle models request: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting Mantle models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMantleCatalogBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading Mantle models response: %w", err)
	}
	if len(body) > maxMantleCatalogBytes {
		return nil, fmt.Errorf("mantle models response exceeds %d bytes", maxMantleCatalogBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 512 {
			detail = detail[:512] + "..."
		}
		if detail == "" {
			detail = "no response body"
		}
		return nil, fmt.Errorf("mantle models returned %s: %s", resp.Status, detail)
	}

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decoding Mantle models response: %w", err)
	}

	models := make([]DiscoveredModel, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		status := strings.ToUpper(model.Status)
		if status == "AVAILABLE" {
			status = "ACTIVE"
		}
		if status == "" {
			// Mantle implements the OpenAI-compatible model-list shape, where
			// presence in the account-scoped response is the availability signal
			// and lifecycle status is normally omitted.
			status = "ACTIVE"
		}
		provider := model.OwnedBy
		if provider == "" {
			provider, _, _ = strings.Cut(model.ID, ".")
		}
		models = append(models, DiscoveredModel{
			ID:           model.ID,
			Status:       status,
			Availability: "AVAILABLE",
			Provider:     provider,
			Source:       SourceMantle,
		})
	}
	return models, nil
}
