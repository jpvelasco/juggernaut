// profile.go — named shared-config profile validation for IAM auth.

package discovery

import (
	"context"
	"fmt"
)

// ValidateProfile reports whether a named shared-config profile can be loaded
// and has a credentials provider — the checks a profile typo fails, without
// contacting AWS. An empty profile means "the ambient default chain will be
// used", which is valid by definition.
//
// Shared-config loading is file I/O only (SSO/assume-role credentials resolve
// lazily), so this never performs a network call. A profile that passes here
// can still fail at request time — expired SSO sessions surface there — but
// this cheap check catches the common breakage: a typo'd or absent profile.
func ValidateProfile(ctx context.Context, region, profile string) error {
	if profile == "" {
		return nil
	}
	cfg, err := loadAWSConfig(ctx, region, profile)
	if err != nil {
		return fmt.Errorf("invalid --profile %q: %w", profile, err)
	}
	if cfg.Credentials == nil {
		return fmt.Errorf("invalid --profile %q: no credentials provider resolved from the shared config", profile)
	}
	return nil
}
