package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/spf13/cobra"
)

var modelsRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Discover the models available to this AWS account and cache them locally",
	RunE:  runModelsRefresh,
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the cached account/region model inventory",
	RunE:  runModelsList,
}

var modelsRefreshFlags struct {
	region string
	source string
}

var modelsListFlags struct {
	region          string
	source          string
	cli             string
	refresh         bool
	showUnsupported bool
}

// These functions are variables so command tests never call real AWS.
var listFoundationCatalog = discovery.ListFoundationModels
var catalogCallerAccount = discovery.CallerAccount
var catalogCredentialScope = discovery.CredentialScope
var catalogNow = time.Now

func init() {
	rf := modelsRefreshCmd.Flags()
	rf.StringVar(&modelsRefreshFlags.region, "region", "us-west-2", "AWS region to query")
	rf.StringVar(&modelsRefreshFlags.source, "source", "all", "catalog source: all, foundation, profile, or native")

	lf := modelsListCmd.Flags()
	lf.StringVar(&modelsListFlags.region, "region", "us-west-2", "AWS region to list")
	lf.StringVar(&modelsListFlags.source, "source", "all", "catalog source: all, foundation, profile, or native")
	lf.StringVar(&modelsListFlags.cli, "cli", "", "show models compatible with this CLI: "+provider.SupportedNames())
	lf.BoolVar(&modelsListFlags.refresh, "refresh", false, "refresh from AWS before listing")
	lf.BoolVar(&modelsListFlags.showUnsupported, "show-unsupported", false, "with --cli, include incompatible models and reasons")

	modelsCmd.AddCommand(modelsRefreshCmd, modelsListCmd)
}

func runModelsRefresh(cmd *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	sources, err := parseCatalogSources(modelsRefreshFlags.source)
	if err != nil {
		return err
	}
	accountID, credentialScope, err := catalogIdentity(cmd.Context(), home, modelsRefreshFlags.region)
	if err != nil {
		return err
	}
	models, err := refreshCatalog(cmd.Context(), modelsRefreshFlags.region, sources)
	if err != nil {
		return err
	}
	refreshedAt := catalogNow()
	if err := discovery.SaveCachedModels(home, accountID, credentialScope, modelsRefreshFlags.region, sources, models, refreshedAt); err != nil {
		return err
	}
	path, err := discovery.CachePath(home)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Cached %d models for AWS account %s in %s at %s\n", len(models), accountID, modelsRefreshFlags.region, path)
	return nil
}

func runModelsList(cmd *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	sources, err := parseCatalogSources(modelsListFlags.source)
	if err != nil {
		return err
	}

	credentialScope, err := catalogCredentialScope(home)
	if err != nil {
		return err
	}
	var snapshot discovery.RegionCatalog
	var found bool
	if modelsListFlags.refresh {
		accountID, accountErr := catalogCallerAccount(cmd.Context(), modelsListFlags.region)
		if accountErr != nil {
			return accountErr
		}
		models, refreshErr := refreshCatalog(cmd.Context(), modelsListFlags.region, sources)
		if refreshErr != nil {
			return refreshErr
		}
		refreshedAt := catalogNow()
		if err := discovery.SaveCachedModels(home, accountID, credentialScope, modelsListFlags.region, sources, models, refreshedAt); err != nil {
			return err
		}
	}
	snapshot, found, err = discovery.LoadCachedModels(home, credentialScope, modelsListFlags.region)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no cached model catalog for %s; run 'juggernaut models refresh --region %s'", modelsListFlags.region, modelsListFlags.region)
	}

	var prov provider.Provider
	if modelsListFlags.cli != "" {
		prov, err = provider.Get(modelsListFlags.cli)
		if err != nil {
			return err
		}
	}
	selectedSources := make(map[discovery.Source]bool, len(sources))
	for _, source := range sources {
		selectedSources[source] = true
	}
	models := append([]discovery.DiscoveredModel(nil), snapshot.Models...)
	sort.Slice(models, func(i, j int) bool {
		if models[i].Source != models[j].Source {
			return models[i].Source < models[j].Source
		}
		return models[i].ID < models[j].ID
	})

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if prov == nil {
		fmt.Fprintln(w, "SOURCE\tSTATUS\tAVAILABILITY\tMODEL")
	} else {
		fmt.Fprintln(w, "SOURCE\tSTATUS\tAVAILABILITY\tSUPPORT\tMODEL\tREASON")
	}
	count := 0
	for _, model := range models {
		if !selectedSources[model.Source] {
			continue
		}
		if prov == nil {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", model.Source, model.Status, model.Availability, model.ID)
			count++
			continue
		}
		support := provider.SupportsCatalogModel(prov, toProviderCatalogModel(model))
		if !support.Supported && !modelsListFlags.showUnsupported {
			continue
		}
		label := "yes"
		if !support.Supported {
			label = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", model.Source, model.Status, model.Availability, label, model.ID, support.Reason)
		count++
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing model list: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%d models for AWS account %s; refreshed %s\n", count, snapshot.AccountID, snapshot.RefreshedAt.Format(time.RFC3339))
	return nil
}

func parseCatalogSources(value string) ([]discovery.Source, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all", "native":
		return []discovery.Source{discovery.SourceFoundation, discovery.SourceProfile}, nil
	case "foundation":
		return []discovery.Source{discovery.SourceFoundation}, nil
	case "profile":
		return []discovery.Source{discovery.SourceProfile}, nil
	default:
		return nil, fmt.Errorf("invalid catalog source %q (expected all, foundation, profile, or native)", value)
	}
}

func refreshCatalog(
	ctx context.Context,
	region string,
	sources []discovery.Source,
) ([]discovery.DiscoveredModel, error) {
	wanted := make(map[discovery.Source]bool, len(sources))
	for _, source := range sources {
		wanted[source] = true
	}
	var models []discovery.DiscoveredModel
	if wanted[discovery.SourceFoundation] {
		found, err := listFoundationCatalog(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("querying Bedrock foundation models: %w", err)
		}
		models = append(models, found...)
	}
	if wanted[discovery.SourceProfile] {
		found, err := listInferenceProfiles(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("querying Bedrock inference profiles: %w", err)
		}
		models = append(models, found...)
	}
	return models, nil
}

func toProviderCatalogModel(model discovery.DiscoveredModel) provider.CatalogModel {
	return provider.CatalogModel{
		ID:           model.ID,
		Status:       model.Status,
		Availability: model.Availability,
		Provider:     model.Provider,
		Source:       string(model.Source),
	}
}

func catalogIdentity(ctx context.Context, home, region string) (accountID, credentialScope string, err error) {
	credentialScope, err = catalogCredentialScope(home)
	if err != nil {
		return "", "", err
	}
	accountID, err = catalogCallerAccount(ctx, region)
	if err != nil {
		return "", "", err
	}
	return accountID, credentialScope, nil
}

// cachedProviderCatalog returns both the provider model list and the refreshed
// source names from the cached snapshot. Sources survive empty catalogs — a
// refresh that returned zero models still records which sources were touched.
// Returns nil/nil when no cache exists (not an error).
func cachedProviderCatalog(home, region string) (models []provider.CatalogModel, sources []string, err error) {
	snapshot, err := loadCachedSnapshot(home, region)
	if err != nil || snapshot == nil {
		return nil, nil, err
	}
	models = make([]provider.CatalogModel, 0, len(snapshot.Models))
	for _, model := range snapshot.Models {
		models = append(models, toProviderCatalogModel(model))
	}
	sources = make([]string, 0, len(snapshot.Sources))
	for _, s := range snapshot.Sources {
		sources = append(sources, string(s))
	}
	return models, sources, nil
}

func loadCachedSnapshot(home, region string) (*discovery.RegionCatalog, error) {
	credentialScope, err := catalogCredentialScope(home)
	if err != nil {
		return nil, err
	}
	snapshot, found, err := discovery.LoadCachedModels(home, credentialScope, region)
	if err != nil || !found {
		return nil, err
	}
	return &snapshot, nil
}
