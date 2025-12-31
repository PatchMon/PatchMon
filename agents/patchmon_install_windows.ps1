# PatchMon Agent Installation Script for Windows (Server-Provided Version)
# This script is designed to be served by the PatchMon server API endpoint
# It will be downloaded and executed via: Invoke-WebRequest ... | Invoke-Expression
# Similar to Linux: curl ... | sh
#
# The server should inject API credentials as environment variables or script content
# Expected variables: $env:PATCHMON_SERVER_URL, $env:PATCHMON_API_ID, $env:PATCHMON_API_KEY

# Script parameters (MUST be first - before any executable code)
# Server can pass these via environment variables (injected before this script runs)
param(
    [string]$ServerURL = $env:PATCHMON_SERVER_URL,
    [string]$APIID = $env:PATCHMON_API_ID,
    [string]$APIKey = $env:PATCHMON_API_KEY,
    [string]$Version = "latest",
    [string]$InstallPath = "C:\Program Files\PatchMon",
    [string]$ConfigPath = "C:\ProgramData\PatchMon"
)

# Exit on any error (must come AFTER param block)
$ErrorActionPreference = "Stop"

# If server injected credentials directly in script (via template replacement), use those
# This allows server to embed credentials directly: $ServerURL = "http://server:port"; $APIID = "..."; $APIKey = "..."
# (Server can replace these values when serving the script)

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Error "This script must be run as Administrator. Right-click PowerShell and select 'Run as Administrator'."
    exit 1
}

Write-Host "PatchMon Agent Installation for Windows" -ForegroundColor Green
Write-Host "=======================================" -ForegroundColor Green

# Determine architecture
$arch = "amd64"
if ([Environment]::Is64BitOperatingSystem) {
    $arch = "amd64"
} else {
    $arch = "386"
}

# Determine download URL
# Priority: 1) Server provides binary, 2) GitHub releases
if ($ServerURL) {
    # Try to download from PatchMon server first (correct endpoint path)
    $downloadURL = "$ServerURL/api/v1/hosts/agent/download?arch=$arch&os=windows&force=binary"
    Write-Host "Downloading from PatchMon server: $downloadURL" -ForegroundColor Cyan
} else {
    # Fallback to GitHub releases
    $repoOwner = "PatchMon"
    $repoName = "PatchMon-agent"
    $baseURL = "https://github.com/${repoOwner}/${repoName}/releases"
    
    if ($Version -eq "latest") {
        $downloadURL = "${baseURL}/latest/download/patchmon-agent-windows-${arch}.exe"
    } else {
        $downloadURL = "${baseURL}/download/v${Version}/patchmon-agent-windows-${arch}.exe"
    }
    Write-Host "Downloading from GitHub: $downloadURL" -ForegroundColor Cyan
}

$binaryName = "patchmon-agent.exe"
$targetPath = Join-Path $InstallPath $binaryName
$tempPath = Join-Path $env:TEMP "patchmon-agent-windows-${arch}.exe"

Write-Host "Architecture: $arch" -ForegroundColor Cyan
Write-Host "Install Path: $InstallPath" -ForegroundColor Cyan
Write-Host "Config Path: $ConfigPath" -ForegroundColor Cyan
Write-Host ""

# Create installation directory
Write-Host "Creating installation directory..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $InstallPath | Out-Null

# Create config directory
Write-Host "Creating configuration directory..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $ConfigPath | Out-Null

# Download the binary
Write-Host "Downloading PatchMon agent..." -ForegroundColor Yellow
try {
    # Use TLS 1.2 for secure downloads
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    
    # Build headers if API credentials are provided (for server downloads)
    $headers = @{}
    if ($APIID -and $APIKey -and $ServerURL) {
        $headers["X-API-ID"] = $APIID
        $headers["X-API-KEY"] = $APIKey
    }
    
    try {
        if ($headers.Count -gt 0) {
            Invoke-WebRequest -Uri $downloadURL -OutFile $tempPath -Headers $headers -UseBasicParsing
        } else {
            Invoke-WebRequest -Uri $downloadURL -OutFile $tempPath -UseBasicParsing
        }
        Write-Host "Download completed from server." -ForegroundColor Green
    } catch {
        # If server download fails (404 or other error), try GitHub as fallback
        if ($ServerURL -and $downloadURL -like "*$ServerURL*") {
            Write-Host "Server download failed (binary may not be available on server), trying GitHub..." -ForegroundColor Yellow
            $githubURL = "https://github.com/PatchMon/PatchMon-agent/releases/latest/download/patchmon-agent-windows-${arch}.exe"
            try {
                Invoke-WebRequest -Uri $githubURL -OutFile $tempPath -UseBasicParsing
                Write-Host "Download completed from GitHub." -ForegroundColor Green
            } catch {
                Write-Error "Failed to download from both server and GitHub: $_"
                throw
            }
        } else {
            throw
        }
    }
} catch {
    Write-Error "Failed to download agent: $_"
    exit 1
}

# Copy to installation directory
Write-Host "Installing agent to $targetPath..." -ForegroundColor Yellow
Copy-Item -Path $tempPath -Destination $targetPath -Force

# Clean up temp file
Remove-Item -Path $tempPath -Force

# Create default config file if it doesn't exist
$configFile = Join-Path $ConfigPath "config.yml"
if (-not (Test-Path $configFile)) {
    Write-Host "Creating default configuration file..." -ForegroundColor Yellow
    $configContent = @"
patchmon_server: "$ServerURL"
api_version: "v1"
credentials_file: "$ConfigPath\credentials.yml"
log_file: "$ConfigPath\patchmon-agent.log"
log_level: "info"
"@
    Set-Content -Path $configFile -Value $configContent
}

# Add to PATH (optional - users can run with full path)
$currentPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine)
if ($currentPath -notlike "*$InstallPath*") {
    Write-Host "Adding PatchMon to system PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$InstallPath", [EnvironmentVariableTarget]::Machine)
    $env:Path = "$env:Path;$InstallPath"
}

# Configure credentials if provided
if ($ServerURL -and $APIID -and $APIKey) {
    Write-Host "Configuring API credentials..." -ForegroundColor Yellow
    & $targetPath config set-api $APIID $APIKey $ServerURL
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Credentials configured successfully." -ForegroundColor Green
    } else {
        Write-Warning "Failed to configure credentials. You can configure them manually later with:"
        Write-Warning "  patchmon-agent.exe config set-api <API_ID> <API_KEY> <SERVER_URL>"
    }
} else {
    Write-Host "" -ForegroundColor Yellow
    Write-Host "No credentials provided. Configure them manually with:" -ForegroundColor Yellow
    Write-Host "  patchmon-agent.exe config set-api <API_ID> <API_KEY> <SERVER_URL>" -ForegroundColor Cyan
}

# Test the installation
Write-Host ""
Write-Host "Testing installation..." -ForegroundColor Yellow
& $targetPath ping
if ($LASTEXITCODE -ne 0) {
    Write-Error "Installation test failed. Please check the installation manually."
    exit 1
}

Write-Host "Installation test successful!" -ForegroundColor Green

# Create and start Windows Service
Write-Host ""
Write-Host "Setting up Windows Service..." -ForegroundColor Yellow

$serviceName = "PatchMonAgent"
$serviceDisplayName = "PatchMon Agent"
$serviceDescription = "PatchMon Agent - Monitors system packages and sends updates to PatchMon server"

# Check if service already exists
$existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

if ($existingService) {
    Write-Host "Service already exists, stopping it..." -ForegroundColor Yellow
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    sc.exe delete $serviceName | Out-Null
    Start-Sleep -Seconds 2
}

# Create the service
Write-Host "Creating Windows Service..." -ForegroundColor Cyan
$servicePath = $targetPath
$serviceArgs = "serve"

try {
    # Use New-Service cmdlet (more reliable than sc.exe in PowerShell)
    $binPathValue = "`"$servicePath`" $serviceArgs"
    
    New-Service -Name $serviceName `
        -BinaryPathName $binPathValue `
        -DisplayName $serviceDisplayName `
        -Description $serviceDescription `
        -StartupType Automatic `
        -ErrorAction Stop | Out-Null
    
    Write-Host "Service created successfully." -ForegroundColor Green
    
    # Start the service
    Write-Host "Starting service..." -ForegroundColor Cyan
    Start-Service -Name $serviceName -ErrorAction Stop
    
    # Wait a moment for service to start
    Start-Sleep -Seconds 3
    
    # Check service status
    $service = Get-Service -Name $serviceName
    if ($service.Status -eq "Running") {
        Write-Host "Service started successfully!" -ForegroundColor Green
    } else {
        Write-Warning "Service was created but is not running. Status: $($service.Status)"
        Write-Host "You can start it manually with: Start-Service -Name $serviceName" -ForegroundColor Yellow
    }
} catch {
    Write-Warning "Failed to create/start Windows Service: $_"
    Write-Host ""
    Write-Host "The agent is installed and configured, but you'll need to run it manually:" -ForegroundColor Yellow
    Write-Host "  patchmon-agent.exe serve" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "To create the service manually later, run as Administrator:" -ForegroundColor Yellow
    Write-Host "  New-Service -Name $serviceName -BinaryPathName '`"$servicePath`" serve' -DisplayName '$serviceDisplayName' -StartupType Automatic" -ForegroundColor Cyan
    Write-Host "  Start-Service -Name $serviceName" -ForegroundColor Cyan
}

Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host ""
Write-Host "Service Status:" -ForegroundColor Cyan
Get-Service -Name $serviceName -ErrorAction SilentlyContinue | Format-Table -AutoSize

Write-Host ""
Write-Host "Useful commands:" -ForegroundColor Cyan
Write-Host "  Get-Service -Name $serviceName          # Check service status" -ForegroundColor Gray
Write-Host "  Start-Service -Name $serviceName        # Start the service" -ForegroundColor Gray
Write-Host "  Stop-Service -Name $serviceName         # Stop the service" -ForegroundColor Gray
Write-Host "  Restart-Service -Name $serviceName      # Restart the service" -ForegroundColor Gray
Write-Host "  patchmon-agent.exe ping                 # Test connectivity" -ForegroundColor Gray
Write-Host "  patchmon-agent.exe report               # Send manual report" -ForegroundColor Gray

