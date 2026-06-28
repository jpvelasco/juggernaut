package bedrock

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// shortTermKeyPrefix is the literal prefix on short-term Bedrock API keys
// produced by the AWS token generator. The remainder is base64 of a SigV4
// presigned URL that carries the key's validity window (X-Amz-Date +
// X-Amz-Expires). Long-term keys use the "ABSK" prefix and carry no expiry.
const shortTermKeyPrefix = "bedrock-" + "api-key-"

// amzDateLayout is the SigV4 X-Amz-Date format (ISO 8601 basic, UTC).
const amzDateLayout = "20060102T150405Z"

// ParseAPIKeyExpiry returns the expiry time embedded in a short-term Bedrock
// API key, or ok=false if the key is not a short-term key or has no parseable
// expiry (e.g. long-term "ABSK" keys). Expiry = X-Amz-Date + X-Amz-Expires.
func ParseAPIKeyExpiry(key string) (time.Time, bool) {
	body, ok := strings.CutPrefix(key, shortTermKeyPrefix)
	if !ok {
		return time.Time{}, false
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return time.Time{}, false
	}
	// The decoded body is a presigned URL (or its query string).
	q, err := url.ParseQuery(extractQuery(string(raw)))
	if err != nil {
		return time.Time{}, false
	}
	amzDate := q.Get("X-Amz-Date")
	amzExpires := q.Get("X-Amz-Expires")
	if amzDate == "" || amzExpires == "" {
		return time.Time{}, false
	}
	issued, err := time.Parse(amzDateLayout, amzDate)
	if err != nil {
		return time.Time{}, false
	}
	secs, err := strconv.Atoi(amzExpires)
	if err != nil {
		return time.Time{}, false
	}
	return issued.Add(time.Duration(secs) * time.Second), true
}

// IsAPIKeyExpired reports whether the key is a short-term key whose embedded
// expiry is at or before now. Keys with no parseable expiry (long-term keys or
// unrecognised formats) are never reported expired.
func IsAPIKeyExpired(key string, now time.Time) bool {
	exp, ok := ParseAPIKeyExpiry(key)
	if !ok {
		return false
	}
	return !now.Before(exp)
}

// extractQuery returns the query portion of a URL string, or the input itself
// if it has no scheme/path (already a bare query string).
func extractQuery(s string) string {
	if _, query, found := strings.Cut(s, "?"); found {
		return query
	}
	return s
}
