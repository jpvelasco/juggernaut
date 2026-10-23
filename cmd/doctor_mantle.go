package cmd

import (
	"fmt"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

// checkProviderMantle validates every selected scope before probing its endpoint.
// The unsigned probe checks reachability only, not account inference permission.
func checkProviderMantle(r *doctor.Report, prov provider.Provider, home, token string, scopes []string) {
	checker, ok := prov.(provider.MantleConfigChecker)
	if !ok {
		return
	}
	for _, scope := range scopes {
		data := readProviderScopeData(prov, home, scope)
		block, found := provider.ReadScopeAuthBlock(prov, home, scope, data)
		if !found {
			continue
		}
		endpoint := provider.BlockEndpoint(block)
		if endpoint == "" || endpoint == bedrock.EndpointRuntime {
			continue
		}
		label := "mantle configuration (" + scope + ")"
		if endpoint != bedrock.EndpointMantle {
			r.Check(label, doctor.Fail, fmt.Sprintf("unknown endpoint %q; re-run apply to repair", endpoint))
			continue
		}
		auth, _ := block["auth"].(map[string]any)
		region, _ := auth["region"].(string)
		if err := checker.ValidateMantleConfig(data, region); err != nil {
			r.Check(label, doctor.Fail, err.Error()+"; re-run `juggernaut apply --cli="+prov.Name()+" --endpoint=mantle` to repair")
			continue
		}
		r.Check(label, doctor.OK, "native mantle Responses routing configured")
		mode := provider.BlockAuthMode(block)
		reportConnectivity(r, "bedrock connectivity (mantle, "+scope+")", checkMantleConnectivity(mode, region, token), mode, region)
	}
}
