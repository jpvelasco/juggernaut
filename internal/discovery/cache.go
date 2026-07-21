package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const catalogCacheVersion = 2

// RegionCatalog is one account-visible catalog snapshot. It is cached so
// apply remains deterministic and offline unless the user explicitly refreshes.
type RegionCatalog struct {
	AccountID   string            `json:"account_id"`
	RefreshedAt time.Time         `json:"refreshed_at"`
	Models      []DiscoveredModel `json:"models"`
}

type accountCatalog struct {
	Regions map[string]RegionCatalog `json:"regions"`
}

type catalogCache struct {
	Version  int                       `json:"version"`
	Accounts map[string]accountCatalog `json:"accounts"`
	Bindings map[string]string         `json:"bindings"`
}

// CachePath returns the owner-local catalog cache path.
func CachePath(home string) (string, error) {
	return safepath.JoinUnder(home, ".juggernaut", "model-catalog.json")
}

// LoadCachedModels loads the region snapshot associated with the current local
// AWS credential scope. A missing cache, binding, account, or region returns
// found=false.
func LoadCachedModels(home, credentialScope, region string) (snapshot RegionCatalog, found bool, err error) {
	cache, found, err := loadCache(home)
	if err != nil || !found {
		return RegionCatalog{}, false, err
	}
	accountID, found := cache.Bindings[credentialScope]
	if !found {
		return RegionCatalog{}, false, nil
	}
	account, found := cache.Accounts[accountID]
	if !found {
		return RegionCatalog{}, false, nil
	}
	snapshot, found = account.Regions[region]
	return snapshot, found, nil
}

// SaveCachedModels replaces only the refreshed sources for one account and
// region, retaining snapshots from other endpoints, accounts, and regions.
func SaveCachedModels(
	home, accountID, credentialScope, region string,
	sources []Source,
	models []DiscoveredModel,
	refreshedAt time.Time,
) error {
	if accountID == "" {
		return fmt.Errorf("catalog cache account ID cannot be empty")
	}
	if credentialScope == "" {
		return fmt.Errorf("catalog cache credential scope cannot be empty")
	}
	if region == "" {
		return fmt.Errorf("catalog cache region cannot be empty")
	}
	if len(sources) == 0 {
		return fmt.Errorf("catalog cache requires at least one refreshed source")
	}

	cache, found, err := loadCache(home)
	if err != nil {
		return err
	}
	if !found {
		cache = catalogCache{Version: catalogCacheVersion}
	}
	if cache.Accounts == nil {
		cache.Accounts = map[string]accountCatalog{}
	}
	if cache.Bindings == nil {
		cache.Bindings = map[string]string{}
	}
	account := cache.Accounts[accountID]
	if account.Regions == nil {
		account.Regions = map[string]RegionCatalog{}
	}

	touched := make(map[Source]bool, len(sources))
	for _, source := range sources {
		touched[source] = true
	}
	merged := make([]DiscoveredModel, 0, len(models))
	if previous, ok := account.Regions[region]; ok {
		for _, model := range previous.Models {
			if !touched[model.Source] {
				merged = append(merged, model)
			}
		}
	}
	for _, model := range models {
		if touched[model.Source] {
			merged = append(merged, model)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Source != merged[j].Source {
			return merged[i].Source < merged[j].Source
		}
		return merged[i].ID < merged[j].ID
	})
	account.Regions[region] = RegionCatalog{
		AccountID:   accountID,
		RefreshedAt: refreshedAt.UTC(),
		Models:      merged,
	}
	cache.Accounts[accountID] = account
	cache.Bindings[credentialScope] = accountID

	encoded, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding model catalog cache: %w", err)
	}
	path, err := CachePath(home)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := safepath.WriteFile(home, tmp, append(encoded, '\n')); err != nil {
		return fmt.Errorf("writing model catalog cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing model catalog cache: %w", err)
	}
	return nil
}

func loadCache(home string) (catalogCache, bool, error) {
	path, err := CachePath(home)
	if err != nil {
		return catalogCache{}, false, err
	}
	data, err := safepath.ReadFile(home, path)
	if errors.Is(err, os.ErrNotExist) {
		return catalogCache{}, false, nil
	}
	if err != nil {
		return catalogCache{}, false, fmt.Errorf("reading model catalog cache: %w", err)
	}
	var cache catalogCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return catalogCache{}, false, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	if cache.Version != catalogCacheVersion {
		return catalogCache{}, false, fmt.Errorf(
			"unsupported model catalog cache version %d (expected %d); run models refresh",
			cache.Version, catalogCacheVersion)
	}
	return cache, true, nil
}
