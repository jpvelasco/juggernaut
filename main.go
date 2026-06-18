// Juggernaut configures Claude Code to use Amazon Bedrock.
package main

import (
	_ "embed"

	"github.com/jpvelasco/juggernaut/v5/cmd"
)

//go:embed bedrock-config.json
var bedrockConfigJSON []byte

func main() {
	cmd.SetEmbeddedConfig(bedrockConfigJSON)
	cmd.Execute()
}
