param(
  [string]$Version = "latest",
  [string]$BinDir = "$env:USERPROFILE\.local\bin"
)

$Repo = "jpvelasco/juggernaut"
$ErrorActionPreference = "Stop"

function Get-LatestVersion {
  $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  return $release.tag_name -replace '^v', ''
}

if ($Version -eq "latest") {
  $Version = Get-LatestVersion
}

$Archive = "juggernaut_windows_amd64.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/v$Version"
$Url = "$BaseUrl/$Archive"
$ChecksumUrl = "$BaseUrl/checksums.txt"

Write-Output "Installing Juggernaut v$Version (windows_amd64)..."

$Tmp = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
  Invoke-WebRequest $Url -OutFile "$Tmp\$Archive"
  Invoke-WebRequest $ChecksumUrl -OutFile "$Tmp\checksums.txt"

  # Verify checksum.
  $Expected = (Get-Content "$Tmp\checksums.txt" | Where-Object { $_ -match [regex]::Escape($Archive) }) -split '\s+' | Select-Object -First 1
  $Actual = (Get-FileHash "$Tmp\$Archive" -Algorithm SHA256).Hash.ToLower()
  if ($Actual -ne $Expected) {
    Write-Error "Checksum mismatch for $Archive. Expected: $Expected, Got: $Actual"
    exit 1
  }

  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  Expand-Archive "$Tmp\$Archive" -DestinationPath $Tmp -Force
  Move-Item "$Tmp\juggernaut.exe" "$BinDir\juggernaut.exe" -Force

} finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

Write-Output "Juggernaut v$Version installed to $BinDir\juggernaut.exe"
Write-Output ""
Write-Output "Next step: juggernaut apply"
