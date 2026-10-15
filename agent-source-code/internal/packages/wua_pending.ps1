function Get-SystemRebootRequired {
    try { return [bool](New-Object -ComObject Microsoft.Update.SystemInfo).RebootRequired } catch { return $false }
}

function Get-InProgressIds($session) {
    $latest = @{}
    try {
        $s = $session.CreateUpdateSearcher()
        $n = $s.GetTotalHistoryCount()
        if ($n -gt 0) {
            foreach ($e in $s.QueryHistory(0, [Math]::Min($n, 500))) {
                if ($e.Operation -ne 1) { continue }
                $k = ([string]$e.UpdateIdentity.UpdateID).ToLower()
                if (-not $latest.ContainsKey($k)) { $latest[$k] = [int]$e.ResultCode }
            }
        }
    } catch {}
    $ids = @{}
    foreach ($k in $latest.Keys) { if ($latest[$k] -eq 1) { $ids[$k] = $true } }
    return $ids
}
