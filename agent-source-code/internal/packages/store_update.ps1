$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Emit($obj) {
    [Console]::Out.WriteLine(($obj | ConvertTo-Json -Compress -Depth 3))
    [Console]::Out.Flush()
}

function Hex($code) {
    if ($null -eq $code) { return '' }
    $u = [BitConverter]::ToUInt32([BitConverter]::GetBytes([int32]$code), 0)
    return ('0x{0:X8}' -f $u)
}

function Get-StoreVersion([string]$pfn) {
    $name = ($pfn -split '_')[0]
    $pkgs = @(Get-AppxPackage -AllUsers -Name $name -ErrorAction SilentlyContinue | Where-Object { $_.PackageFamilyName -eq $pfn -and -not $_.IsResourcePackage })
    if ($pkgs.Count -eq 0) { return @{ version = ''; installed = $false } }
    $top = $pkgs | Sort-Object { [version]$_.Version } | Select-Object -Last 1
    $installed = @($top.PackageUserInformation | Where-Object { [string]$_.InstallState -eq 'Installed' }).Count -gt 0
    return @{ version = [string]$top.Version; installed = $installed }
}

$req = [Console]::In.ReadToEnd() | ConvertFrom-Json
$target = [string]$req.target
$deadline = (Get-Date).AddSeconds([int]$req.seconds)

try {
    Add-Type -AssemblyName System.Runtime.WindowsRuntime
    $null = [Windows.ApplicationModel.Store.Preview.InstallControl.AppInstallManager, Windows.ApplicationModel.Store.Preview, ContentType = WindowsRuntime]
    $mgr = New-Object Windows.ApplicationModel.Store.Preview.InstallControl.AppInstallManager
    $itemType = [Type]'Windows.ApplicationModel.Store.Preview.InstallControl.AppInstallItem, Windows.ApplicationModel.Store.Preview, ContentType = WindowsRuntime'
    $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
        $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
    } | Select-Object -First 1
} catch {
    Emit @{ state = 'Unavailable'; error = $_.Exception.Message }
    exit 0
}

try {
    $isProductId = $target -match '^(9|XP)[0-9A-Z]{10,13}$'
    $pfn = ''
    $before = @{ version = ''; installed = $false }
    if ($isProductId) {
        $op = $mgr.SearchForUpdatesAsync($target, '')
    } else {
        $pfn = $target
        $before = Get-StoreVersion $pfn
        $op = $mgr.UpdateAppByPackageFamilyNameAsync($pfn)
    }
    $task = $asTask.MakeGenericMethod($itemType).Invoke($null, @($op))
    $waitMs = [int][Math]::Max(1000, ($deadline - (Get-Date)).TotalMilliseconds)
    if (-not $task.Wait($waitMs)) {
        Emit @{ state = 'Searching'; pfn = $pfn; before = $before.version }
        exit 0
    }
    $item = $task.Result
    if ($null -eq $item) {
        Emit @{ state = 'NoUpdate'; pfn = $pfn; before = $before.version }
        exit 0
    }
    if ($isProductId) {
        $pfn = [string]$item.PackageFamilyName
        $before = Get-StoreVersion $pfn
    }

    $nudged = [datetime]::MinValue
    while ($true) {
        $status = $item.GetCurrentStatus()
        $state = [string]$status.InstallState
        if ($state -in 'Completed', 'Error', 'Canceled' -or $state -like 'Paused*') { break }
        if ((Get-Date) -ge $deadline) { break }
        if ($state -eq 'ReadyToDownload' -and ((Get-Date) - $nudged).TotalSeconds -ge 15) {
            $mgr.MoveToFrontOfDownloadQueue($item.ProductId, '')
            $nudged = Get-Date
        }
        Start-Sleep -Seconds 2
    }

    $code = ''
    if ($null -ne $status.ErrorCode) { $code = Hex $status.ErrorCode.HResult }
    $after = Get-StoreVersion $pfn
    Emit @{
        state            = $state
        pfn              = $pfn
        errorCode        = $code
        before           = $before.version
        after            = $after.version
        installedForUser = $after.installed
    }
} catch {
    $e = $_.Exception
    while ($e.InnerException) { $e = $e.InnerException }
    Emit @{ state = 'Error'; pfn = $pfn; error = $e.Message; errorCode = (Hex $e.HResult) }
}
