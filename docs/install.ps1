# Implemented based on Dapr Cli https://github.com/dapr/cli/tree/master/install

param (
    [string]$Version,
    [string]$VelaRoot = "c:\vela"
)

Write-Output ""
$ErrorActionPreference = 'stop'

#Escape space of VelaRoot path
$VelaRoot = $VelaRoot -replace ' ', '` '

# Constants
$VelaCliFileName = "vela.exe"
$VelaCliFilePath = "${VelaRoot}\${VelaCliFileName}"

# GitHub Org and repo hosting Vela CLI
$GitHubOrg = "oam-dev"
$GitHubRepo = "kubevela"

# Set Github request authentication for basic authentication.
if ($Env:GITHUB_USER) {
    $basicAuth = [System.Convert]::ToBase64String([System.Text.Encoding]::ASCII.GetBytes($Env:GITHUB_USER + ":" + $Env:GITHUB_TOKEN));
    $githubHeader = @{"Authorization" = "Basic $basicAuth" }
}
else {
    $githubHeader = @{}
}

if ((Get-ExecutionPolicy) -gt 'RemoteSigned' -or (Get-ExecutionPolicy) -eq 'ByPass') {
    Write-Output "PowerShell requires an execution policy of 'RemoteSigned'."
    Write-Output "To make this change please run:"
    Write-Output "'Set-ExecutionPolicy RemoteSigned -scope CurrentUser'"
    break
}

# Change security protocol to support TLS 1.2 / 1.1 / 1.0 - old powershell uses TLS 1.0 as a default protocol
[Net.ServicePointManager]::SecurityProtocol = "tls12, tls11, tls"

# Check if KubeVela CLI is installed.
if (Test-Path $VelaCliFilePath -PathType Leaf) {
    Write-Warning "vela is detected - $VelaCliFilePath"
    Invoke-Expression "$VelaCliFilePath --version"
    Write-Output "Reinstalling KubeVela..."
}
else {
    Write-Output "Installing Vela..."
}

# Create Vela Directory
Write-Output "Creating $VelaRoot directory"
New-Item -ErrorAction Ignore -Path $VelaRoot -ItemType "directory"
if (!(Test-Path $VelaRoot -PathType Container)) {
    throw "Cannot create $VelaRoot"
}

# Get the list of release from GitHub
$releases = Invoke-RestMethod -Headers $githubHeader -Uri "https://api.github.com/repos/${GitHubOrg}/${GitHubRepo}/releases" -Method Get
if ($releases.Count -eq 0) {
    throw "No releases from github.com/oam-dev/kubevela repo"
}

# Filter windows binary and download archive
$os_arch = "windows-amd64"
if (!$Version) {
    $windowsAsset = $releases | Where-Object { $_.tag_name -notlike "*rc*" } | Select-Object -First 1 | Select-Object -ExpandProperty assets | Where-Object { $_.name -Like "*${os_arch}.zip" }
    if (!$windowsAsset) {
        throw "Cannot find the windows KubeVela CLI binary"
    }
    $zipFileUrl = $windowsAsset.url
    $assetName = $windowsAsset.name
} else {
    $assetName = "vela-${Version}-${os_arch}.zip"
    $zipFileUrl = "https://github.com/${GitHubOrg}/${GitHubRepo}/releases/download/${Version}/${assetName}"
}

$zipFilePath = $VelaRoot + "\" + $assetName
Write-Output "Downloading $zipFileUrl ..."

$githubHeader.Accept = "application/octet-stream"
Invoke-WebRequest -Headers $githubHeader -Uri $zipFileUrl -OutFile $zipFilePath
if (!(Test-Path $zipFilePath -PathType Leaf)) {
    throw "Failed to download Vela Cli binary - $zipFilePath"
}

# Extract KubeVela CLI to $VelaRoot
Write-Output "Extracting $zipFilePath..."
Microsoft.Powershell.Archive\Expand-Archive -Force -Path $zipFilePath -DestinationPath $VelaRoot
$ExtractedVelaCliFilePath = "${VelaRoot}\${os_arch}\${VelaCliFileName}"
Copy-Item $ExtractedVelaCliFilePath -Destination $VelaCliFilePath
if (!(Test-Path $VelaCliFilePath -PathType Leaf)) {
    throw "Failed to extract Vela Cli archive - $zipFilePath"
}

# Check the KubeVela CLI version
Invoke-Expression "$VelaCliFilePath --version"

# Clean up zipfile
Write-Output "Clean up $zipFilePath..."
Remove-Item $zipFilePath -Force

# Add VelaRoot directory to User Path environment variable
Write-Output "Try to add $VelaRoot to User Path Environment variable..."
$UserPathEnvironmentVar = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPathEnvironmentVar -like '*vela*') {
    Write-Output "Skipping to add $VelaRoot to User Path - $UserPathEnvironmentVar"
}
else {
    [System.Environment]::SetEnvironmentVariable("PATH", $UserPathEnvironmentVar + ";$VelaRoot", "User")
    $UserPathEnvironmentVar = [Environment]::GetEnvironmentVariable("PATH", "User")
    Write-Output "Added $VelaRoot to User Path - $UserPathEnvironmentVar"
}

Write-Output "`r`nKubeVela CLI is installed successfully."
Write-Output "To get started with KubeVela, please visit https://kubevela.io."
