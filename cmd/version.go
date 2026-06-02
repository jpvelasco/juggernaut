package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var versionJSONFlag bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Juggernaut version",
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionJSONFlag {
			out, _ := json.Marshal(map[string]string{"version": Version})
			fmt.Println(string(out))
			return nil
		}
		fmt.Println(Version)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSONFlag, "json", false, "output as JSON")
	rootCmd.AddCommand(versionCmd)
}
