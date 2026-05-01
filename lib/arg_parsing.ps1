# lib/arg_parsing.ps1 — Shared GNU-style argument parser for PowerShell scripts.
# Extracted from juggernaut.ps1. Dot-source this file; do not run directly.

# Convert-GnuStyleArgs
# Converts an array of GNU-style CLI arguments (--flag, --key=value, --key value,
# -k, -k value) into a hashtable suitable for splatting into a PowerShell script.
# Unknown positional args collect under key 'RemainingArgs'.
function Convert-GnuStyleArgs {
    param([string[]]$InputArgs)
    $converted = @{}
    for ($i = 0; $i -lt $InputArgs.Count; $i++) {
        $arg = [string]$InputArgs[$i]
        switch -Regex ($arg) {
            '^--([^=]+)=(.*)$' {
                $rawName = $Matches[1]
                $value = $Matches[2]
                $name = $rawName -replace '-', ''
                $converted[$name] = $value
                continue
            }
            '^--(.+)$' {
                $rawName = $Matches[1]
                $name = $rawName -replace '-', ''
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            '^-([^=]+)=(.*)$' {
                $rawName = $Matches[1]
                $value = $Matches[2]
                $name = $rawName -replace '-', ''
                $converted[$name] = $value
                continue
            }
            '^-([^-].*)$' {
                $rawName = $Matches[1]
                if ($rawName -eq 'h') { $name = 'Help' }
                elseif ($rawName -eq 'v') { $name = 'Version' }
                else { $name = $rawName -replace '-', '' }
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            default {
                if (-not $converted.ContainsKey('RemainingArgs')) {
                    $converted['RemainingArgs'] = @()
                }
                $converted['RemainingArgs'] += $arg
            }
        }
    }
    return $converted
}
