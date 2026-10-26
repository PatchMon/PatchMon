if ($env:PROCESSOR_ARCHITEW6432) {
    & "$env:SystemRoot\Sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File $PSCommandPath
    exit $LASTEXITCODE
}

$ErrorActionPreference = "Stop"
$Server = "https://patchmon.example.com"
$Key    = "patchmon_ae_xxxxxxxx"
$Secret = "xxxxxxxx"

$Server     = $Server.TrimEnd("/")
$svcName    = "PatchMonAgent"
$dataDir    = Join-Path $env:ProgramData "PatchMon"
$credsFile  = Join-Path $dataDir "credentials.yml"
$configFile = Join-Path $dataDir "config.yml"
$hostIdFile = Join-Path $dataDir "intune-host-id"
$installer  = Join-Path $env:TEMP "patchmon-install.ps1"
$logDir     = Join-Path $env:ProgramData "Microsoft\IntuneManagementExtension\Logs"
$retry      = 1618
$failed     = 1

if (Test-Path $logDir) { Start-Transcript -Path (Join-Path $logDir "PatchMonInstall.log") -Append | Out-Null }

function Exit-Install($code) {
    if (Test-Path $logDir) { Stop-Transcript | Out-Null }
    exit $code
}

function Wait-Running {
    $svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
    if (-not $svc) { return $false }
    try {
        if ($svc.Status -ne "Running") { Start-Service -Name $svcName }
        $svc.WaitForStatus("Running", "00:00:30")
        return $true
    } catch {
        Write-Host "Service $svcName did not reach Running: $($_.Exception.Message)"
        return $false
    }
}

function Get-StatusCode($err) {
    if ($err.Exception.Response) { return [int]$err.Exception.Response.StatusCode }
    return 0
}

function Get-Installer($apiId, $apiKey) {
    try {
        Invoke-WebRequest -Uri "$Server/api/v1/hosts/install?os=windows" -UseBasicParsing -OutFile $installer `
            -Headers @{ "X-API-ID" = $apiId; "X-API-KEY" = $apiKey }
        return 200
    } catch {
        Write-Host "Installer download failed: $($_.Exception.Message)"
        return (Get-StatusCode $_)
    }
}

if (Wait-Running) { Exit-Install 0 }

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$haveInstaller = $false
if (Test-Path $credsFile) {
    $yaml = Get-Content -Path $credsFile -Raw
    if ($yaml -match '(?m)^api_id:\s*(\S+)') { $savedId = $Matches[1].Trim("'`"") }
    if ($yaml -match '(?m)^api_key:\s*(\S+)') { $savedKey = $Matches[1].Trim("'`"") }
    if ($savedId -and $savedKey) {
        $status = Get-Installer $savedId $savedKey
        if ($status -eq 200) { $haveInstaller = $true }
        elseif ($status -ne 401) { Exit-Install $retry }
    }
}

if (-not $haveInstaller) {
    $hklm = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine, [Microsoft.Win32.RegistryView]::Registry64)
    $machineId = $hklm.OpenSubKey("SOFTWARE\Microsoft\Cryptography").GetValue("MachineGuid").ToLower()

    $enrol = @{ friendly_name = $env:COMPUTERNAME; machine_id = $machineId }
    if (Test-Path $hostIdFile) {
        $enrol.host_id = (Get-Content -Path $hostIdFile -Raw).Trim()
    } elseif ((Test-Path $configFile) -and ((Get-Content -Path $configFile -Raw) -match "(?m)^host_id:\s*[`"']?([A-Za-z0-9-]+)")) {
        $enrol.host_id = $Matches[1]
    }

    try {
        $r = Invoke-RestMethod -Method Post -Uri "$Server/api/v1/auto-enrollment/enroll" -UseBasicParsing `
            -Headers @{ "X-Auto-Enrollment-Key" = $Key; "X-Auto-Enrollment-Secret" = $Secret } `
            -ContentType "application/json" -Body ($enrol | ConvertTo-Json)
    } catch {
        $status = Get-StatusCode $_
        $detail = if ($_.ErrorDetails -and $_.ErrorDetails.Message) { $_.ErrorDetails.Message } else { $_.Exception.Message }
        Write-Host "PatchMon enrolment failed (HTTP $status): $detail"
        if ($status -in 400, 401, 403, 409) { Exit-Install $failed }
        Exit-Install $retry
    }

    New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
    Set-Content -Path $hostIdFile -Value $r.host.id -Encoding ASCII

    if ((Get-Installer $r.host.api_id $r.host.api_key) -ne 200) { Exit-Install $retry }
}

$ErrorActionPreference = "Continue"
& "$PSHOME\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File $installer 2>&1 | ForEach-Object { "$_" } | Out-Host
$code = $LASTEXITCODE
$ErrorActionPreference = "Stop"
Remove-Item -Path $installer -Force -ErrorAction SilentlyContinue
if ($code -ne 0) {
    Write-Host "PatchMon installer exited with $code"
    Exit-Install $retry
}

if (-not (Wait-Running)) { Exit-Install $retry }

Remove-Item -Path $hostIdFile -Force -ErrorAction SilentlyContinue
Exit-Install 0
