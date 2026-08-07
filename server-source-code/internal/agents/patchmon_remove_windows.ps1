# PatchMon Agent Removal Script for Windows
# This script completely removes PatchMon from the Windows system
# Usage: Run PowerShell as Administrator, then:
#        .\patchmon_remove_windows.ps1
#        .\patchmon_remove_windows.ps1 -RemoveAll
#        .\patchmon_remove_windows.ps1 -RemoveConfig -RemoveLogs -Force

# Script parameters
param(
    [switch]$RemoveConfig = $false,    # Remove configuration and credentials files
    [switch]$RemoveLogs = $false,      # Remove log files
    [switch]$RemoveAll = $false,       # Shortcut: Remove all files (config + logs)
    [switch]$Force = $false,           # Skip confirmation prompts
    [string]$InstallPath = "C:\Program Files\PatchMon",
    [string]$ConfigPath = "C:\ProgramData\PatchMon"
)

# If RemoveAll is specified, set both RemoveConfig and RemoveLogs
if ($RemoveAll) {
    $RemoveConfig = $true
    $RemoveLogs = $true
}

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Error "This script must be run as Administrator. Right-click PowerShell and select 'Run as Administrator'."
    exit 1
}

# Service name
$serviceName = "PatchMonAgent"

# Functions
function Write-Info {
    param([string]$Message)
    Write-Host "ℹ️  $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "✅ $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "⚠️  $Message" -ForegroundColor Yellow
}

function Write-Error-Msg {
    param([string]$Message)
    Write-Host "❌ ERROR: $Message" -ForegroundColor Red
}

Write-Host ""
Write-Info "🗑️  Starting PatchMon Agent Removal..."
Write-Host ""

# Step 1: Stop and remove Windows Service
Write-Info "🛑 Stopping PatchMon service..."
$serviceStopped = $false

try {
    $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($service) {
        if ($service.Status -eq "Running") {
            Write-Warn "Service is running. Stopping it now..."
            Stop-Service -Name $serviceName -Force -ErrorAction Stop
            Start-Sleep -Seconds 2

            # Verify it stopped
            $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
            if ($service.Status -eq "Running") {
                Write-Warn "⚠️  Service is STILL RUNNING after stop command! Attempting force stop..."
                & sc.exe stop $serviceName | Out-Null
                Start-Sleep -Seconds 2
            } else {
                Write-Success "✓ Service stopped successfully"
            }
            $serviceStopped = $true
        } else {
            Write-Info "Service exists but is not running (Status: $($service.Status))"
            $serviceStopped = $true
        }

        # Remove the service
        Write-Info "Removing Windows Service..."
        & sc.exe delete $serviceName 2>&1 | Out-Null
        Start-Sleep -Seconds 2

        # Verify service was removed
        $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
        if (-not $service) {
            Write-Success "✓ Service removed successfully"
        } else {
            Write-Warn "⚠️  Service still exists after deletion attempt"
        }
    } else {
        Write-Info "Windows Service not found"
    }
} catch {
    Write-Warn "Error managing service: $_"
}

# Step 2: Stop any running agent processes
Write-Info "🔄 Stopping any running PatchMon processes..."
$processes = Get-Process -Name "patchmon-agent" -ErrorAction SilentlyContinue
if ($processes) {
    Write-Warn "Found $($processes.Count) running PatchMon process(es), stopping them..."
    $processes | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2

    # Verify they stopped
    $remaining = Get-Process -Name "patchmon-agent" -ErrorAction SilentlyContinue
    if ($remaining) {
        Write-Warn "⚠️  Some processes are still running after stop command"
    } else {
        Write-Success "✓ All processes stopped"
    }
} else {
    Write-Info "No running PatchMon processes found"
}

# Step 3: Remove agent binary
Write-Info "📄 Removing agent binaries..."
$binaryRemoved = $false

$binaryPath = Join-Path $InstallPath "patchmon-agent.exe"
if (Test-Path $binaryPath) {
    Write-Warn "Removing agent binary: $binaryPath"
    try {
        $process = Get-Process -Name "patchmon-agent" -ErrorAction SilentlyContinue
        if ($process) {
            Write-Warn "Binary is in use by a process, waiting..."
            Start-Sleep -Seconds 3
        }
        Remove-Item -Path $binaryPath -Force -ErrorAction Stop
        Write-Success "✓ Binary removed"
        $binaryRemoved = $true
    } catch {
        Write-Warn "Failed to remove binary: $_"
        Write-Warn "You may need to manually delete: $binaryPath"
    }
} else {
    Write-Info "Agent binary not found at: $binaryPath"
}

# Remove backup binaries
$backupBinaries = Get-ChildItem -Path $InstallPath -Filter "patchmon-agent*.backup.*" -ErrorAction SilentlyContinue
if ($backupBinaries) {
    Write-Warn "Removing $($backupBinaries.Count) backup binary file(s)..."
    $backupBinaries | Remove-Item -Force -ErrorAction SilentlyContinue
    $binaryRemoved = $true
}

if ($binaryRemoved) {
    Write-Success "Agent binaries removed"
} else {
    Write-Info "No agent binaries found to remove"
}

# Step 4: Remove from PATH
Write-Info "🗑️  Removing PatchMon from system PATH..."
try {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine)
    if ($currentPath -like "*$InstallPath*") {
        Write-Warn "Removing PatchMon from system PATH..."
        $newPath = ($currentPath -split ';' | Where-Object { $_ -ne $InstallPath }) -join ';'
        [Environment]::SetEnvironmentVariable("Path", $newPath, [EnvironmentVariableTarget]::Machine)
        Write-Success "✓ Removed from PATH"
    } else {
        Write-Info "PatchMon not found in system PATH"
    }
} catch {
    Write-Warn "Failed to remove from PATH: $_"
}

# Step 5: Remove installation directory (if empty or RemoveAll)
Write-Info "📁 Checking installation directory..."
if (Test-Path $InstallPath) {
    $items = Get-ChildItem -Path $InstallPath -ErrorAction SilentlyContinue
    if ($items.Count -eq 0 -or $RemoveAll) {
        Write-Warn "Removing installation directory: $InstallPath"
        try {
            Remove-Item -Path $InstallPath -Recurse -Force -ErrorAction Stop
            Write-Success "✓ Installation directory removed"
        } catch {
            Write-Warn "Failed to remove installation directory: $_"
            Write-Warn "You may need to manually delete: $InstallPath"
        }
    } else {
        Write-Info "Installation directory contains files, leaving it (use -RemoveAll to force removal)"
    }
} else {
    Write-Info "Installation directory not found"
}

# Step 6: Remove configuration files (optional)
if ($RemoveConfig) {
    Write-Info "📋 Removing configuration files..."
    if (Test-Path $ConfigPath) {
        Write-Warn "Removing configuration directory: $ConfigPath"
        try {
            Remove-Item -Path $ConfigPath -Recurse -Force -ErrorAction Stop
            Write-Success "✓ Configuration directory removed"
        } catch {
            Write-Warn "Failed to remove configuration directory: $_"
            Write-Warn "You may need to manually delete: $ConfigPath"
        }
    } else {
        Write-Info "Configuration directory not found"
    }
} else {
    Write-Info "📋 Configuration files preserved (use -RemoveConfig to remove)"
    if (Test-Path $ConfigPath) {
        Write-Host "   Location: $ConfigPath" -ForegroundColor Gray
    }
}

# Step 7: Remove log files (optional)
if ($RemoveLogs) {
    Write-Info "📝 Removing log files..."
    $logPath = Join-Path $ConfigPath "patchmon-agent.log"
    if (Test-Path $logPath) {
        Write-Warn "Removing log file: $logPath"
        try {
            Remove-Item -Path $logPath -Force -ErrorAction Stop
            Write-Success "✓ Log file removed"
        } catch {
            Write-Warn "Failed to remove log file: $_"
        }
    } else {
        Write-Info "Log file not found"
    }

    # Remove log backups
    $logBackups = Get-ChildItem -Path $ConfigPath -Filter "patchmon-agent.log.old.*" -ErrorAction SilentlyContinue
    if ($logBackups) {
        Write-Warn "Removing $($logBackups.Count) log backup file(s)..."
        $logBackups | Remove-Item -Force -ErrorAction SilentlyContinue
    }
} else {
    Write-Info "📝 Log files preserved (use -RemoveLogs to remove)"
}

# Step 8: Clean up backup files in config directory (if RemoveConfig or RemoveAll)
if ($RemoveConfig -or $RemoveAll) {
    Write-Info "🧹 Checking for backup files..."
    $backupFiles = @()

    if (Test-Path $ConfigPath) {
        $backupFiles += Get-ChildItem -Path $ConfigPath -Filter "*.backup.*" -ErrorAction SilentlyContinue
    }

    if ($backupFiles.Count -gt 0) {
        Write-Warn "Removing $($backupFiles.Count) backup file(s)..."
        $backupFiles | Remove-Item -Force -ErrorAction SilentlyContinue
        Write-Success "✓ Backup files removed"
    } else {
        Write-Info "No backup files found"
    }
}

# Final summary
Write-Host ""
Write-Success "Removal process complete!"
Write-Host ""
Write-Info "Summary of actions taken:"
Write-Host "  • Windows Service: Stopped and removed" -ForegroundColor Gray
Write-Host "  • Agent binaries: Removed" -ForegroundColor Gray
Write-Host "  • System PATH: Updated" -ForegroundColor Gray
if ($RemoveConfig) {
    Write-Host "  • Configuration files: Removed" -ForegroundColor Gray
} else {
    Write-Host "  • Configuration files: Preserved" -ForegroundColor Gray
}
if ($RemoveLogs) {
    Write-Host "  • Log files: Removed" -ForegroundColor Gray
} else {
    Write-Host "  • Log files: Preserved" -ForegroundColor Gray
}

if (-not $RemoveAll) {
    Write-Host ""
    Write-Info "To completely remove config and logs, run with -RemoveAll"
    Write-Host "  .\patchmon_remove_windows.ps1 -RemoveAll -Force" -ForegroundColor Cyan
}

Write-Host ""
