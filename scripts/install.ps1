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

$Platform = "windows_amd64"
$TarArchive = "juggernaut_$Platform.tar.gz"
$ZipArchive = "juggernaut_$Platform.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/v$Version"
$ChecksumUrl = "$BaseUrl/checksums.txt"

Write-Output "Installing Juggernaut v$Version ($Platform)..."

$Tmp = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
  Invoke-WebRequest $ChecksumUrl -OutFile "$Tmp\checksums.txt"
  $Checksums = Get-Content "$Tmp\checksums.txt" -Raw

  $ArchiveKind = $null
  $Archive = $null
  if ($Checksums -match [regex]::Escape($TarArchive)) {
    $Archive = $TarArchive
    $ArchiveKind = "tar.gz"
  } elseif ($Checksums -match [regex]::Escape($ZipArchive)) {
    $Archive = $ZipArchive
    $ArchiveKind = "zip"
  } else {
    Write-Error "No supported archive found in release checksums for $Platform"
    exit 1
  }

  $Url = "$BaseUrl/$Archive"
  Invoke-WebRequest $Url -OutFile "$Tmp\$Archive"

  # Verify checksum.
  $Expected = (Get-Content "$Tmp\checksums.txt" | Where-Object { $_ -match [regex]::Escape($Archive) }) -split '\s+' | Select-Object -First 1
  $Actual = (Get-FileHash "$Tmp\$Archive" -Algorithm SHA256).Hash.ToLower()
  if ($Actual -ne $Expected) {
    Write-Error "Checksum mismatch for $Archive. Expected: $Expected, Got: $Actual"
    exit 1
  }

  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  if ($ArchiveKind -eq "zip") {
    Expand-Archive -LiteralPath "$Tmp\$Archive" -DestinationPath $BinDir -Force
  } else {
    tar -xzf "$Tmp\$Archive" -C $BinDir
  }

} finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

Write-Output "Juggernaut v$Version installed to $BinDir\juggernaut.exe"
Write-Output ""
Write-Output "Next step: juggernaut apply"
