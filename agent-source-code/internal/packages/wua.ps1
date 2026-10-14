$ErrorActionPreference = 'Stop'
$guidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\z'

function Emit($obj) {
    [Console]::Out.WriteLine(($obj | ConvertTo-Json -Compress -Depth 4))
    [Console]::Out.Flush()
}

function Hex($code) {
    if ($null -eq $code) { return '' }
    $u = [BitConverter]::ToUInt32([BitConverter]::GetBytes([int32]$code), 0)
    return ('0x{0:X8}' -f $u)
}

function Find-Updates($session, $ids, $online) {
    $s = $session.CreateUpdateSearcher()
    $s.Online = $online
    $crit = ($ids | ForEach-Object { "UpdateID='$_'" }) -join ' OR '
    $map = @{}
    foreach ($u in $s.Search($crit).Updates) {
        $k = ([string]$u.Identity.UpdateID).ToLower()
        if (-not $map.ContainsKey($k) -or -not $u.IsInstalled) { $map[$k] = $u }
    }
    return $map
}

function Find-ById($session, $ids) {
    $map = @{}
    try { $map = Find-Updates $session $ids $false } catch {}
    $missing = @($ids | Where-Object { -not $map.ContainsKey($_.ToLower()) })
    if ($missing.Count -gt 0) {
        $more = Find-Updates $session $missing $true
        foreach ($k in $more.Keys) { $map[$k] = $more[$k] }
    }
    return $map
}

function Invoke-Scan($session) {
    $searcher = $session.CreateUpdateSearcher()
    $res = $searcher.Search('IsInstalled=0 AND IsHidden=0')
    $inProgress = @{}
    if (Get-SystemRebootRequired) { $inProgress = Get-InProgressIds $session }
    foreach ($u in $res.Updates) {
        $id = ([string]$u.Identity.UpdateID).ToLower()
        $ib = $u.InstallationBehavior
        Emit @{
            type                   = 'update'
            id                     = $id
            revision               = [int]$u.Identity.RevisionNumber
            title                  = [string]$u.Title
            kbs                    = @($u.KBArticleIDs | ForEach-Object { "KB$_" })
            categories             = @($u.Categories | ForEach-Object { [string]$_.Name })
            category_ids           = @($u.Categories | ForEach-Object { [string]$_.CategoryID })
            type_code              = [int]$u.Type
            browse_only            = [bool]$u.BrowseOnly
            is_downloaded          = [bool]$u.IsDownloaded
            reboot_behavior        = [int]$ib.RebootBehavior
            can_request_user_input = [bool]$ib.CanRequestUserInput
            pending_reboot         = [bool]$inProgress.ContainsKey($id)
        }
    }
    $before = $false
    try { $before = [bool]$session.CreateUpdateInstaller().RebootRequiredBeforeInstallation } catch {}
    Emit @{ type = 'result'; op = 'scan'; count = [int]$res.Updates.Count; reboot_required_before_install = $before }
}

function Invoke-Download($session, $ids) {
    $map = Find-ById $session $ids
    $coll = New-Object -ComObject Microsoft.Update.UpdateColl
    $order = @()
    foreach ($id in $ids) {
        $u = $map[$id.ToLower()]
        if ($null -eq $u -or $u.IsInstalled -or $u.IsDownloaded) { continue }
        if (-not $u.EulaAccepted) { $u.AcceptEula() }
        [void]$coll.Add($u)
        $order += $id.ToLower()
    }
    if ($coll.Count -eq 0) {
        Emit @{ type = 'result'; op = 'download'; result_code = 2; hresult = '' }
        return
    }
    $dl = $session.CreateUpdateDownloader()
    $dl.Updates = $coll
    $r = $dl.Download()
    for ($i = 0; $i -lt $coll.Count; $i++) {
        $ur = $r.GetUpdateResult($i)
        Emit @{ type = 'update_result'; id = $order[$i]; found = $true; result_code = [int]$ur.ResultCode; hresult = (Hex $ur.HResult) }
    }
    Emit @{ type = 'result'; op = 'download'; result_code = [int]$r.ResultCode; hresult = (Hex $r.HResult) }
}

function Invoke-Status($session, $id) {
    $id = $id.ToLower()
    $map = Find-ById $session @($id)
    $u = $map[$id]
    if ($null -eq $u) {
        Emit @{ type = 'result'; op = 'status'; id = $id; found = $false }
        return
    }
    $pending = $false
    if (-not $u.IsInstalled -and (Get-SystemRebootRequired)) { $pending = (Get-InProgressIds $session).ContainsKey($id) }
    Emit @{ type = 'result'; op = 'status'; id = $id; found = $true; already_installed = [bool]$u.IsInstalled; pending_reboot = $pending }
}

function Invoke-Install($session, $id) {
    $id = $id.ToLower()
    $map = Find-ById $session @($id)
    $u = $map[$id]
    if ($null -eq $u) {
        Emit @{ type = 'result'; op = 'install'; id = $id; found = $false }
        return
    }
    if ($u.IsInstalled) {
        Emit @{ type = 'result'; op = 'install'; id = $id; found = $true; already_installed = $true }
        return
    }
    if (Get-SystemRebootRequired) {
        if ((Get-InProgressIds $session).ContainsKey($id)) {
            Emit @{ type = 'result'; op = 'install'; id = $id; found = $true; pending_reboot = $true }
            return
        }
    }
    if (-not $u.EulaAccepted) { $u.AcceptEula() }
    $coll = New-Object -ComObject Microsoft.Update.UpdateColl
    [void]$coll.Add($u)
    if (-not $u.IsDownloaded) {
        $dl = $session.CreateUpdateDownloader()
        $dl.Updates = $coll
        $dr = $dl.Download()
        $dur = $dr.GetUpdateResult(0)
        if ([int]$dur.ResultCode -ne 2 -and [int]$dur.ResultCode -ne 3) {
            Emit @{ type = 'result'; op = 'install'; id = $id; found = $true; result_code = 4; hresult = (Hex $dur.HResult) }
            return
        }
    }
    $inst = $session.CreateUpdateInstaller()
    try { $inst.ForceQuiet = $true } catch {}
    try { $inst.AllowSourcePrompts = $false } catch {}
    $inst.Updates = $coll
    $r = $inst.Install()
    $ur = $r.GetUpdateResult(0)
    Emit @{
        type            = 'result'
        op              = 'install'
        id              = $id
        found           = $true
        result_code     = [int]$ur.ResultCode
        hresult         = (Hex $ur.HResult)
        reboot_required = ([bool]$ur.RebootRequired -or [bool]$r.RebootRequired)
    }
}

try {
    $req = [Console]::In.ReadToEnd() | ConvertFrom-Json
    $session = New-Object -ComObject Microsoft.Update.Session
    try { $session.ClientApplicationID = 'PatchMon' } catch {}
    switch ([string]$req.op) {
        'scan' { Invoke-Scan $session }
        'download' {
            $ids = @($req.ids | Where-Object { ([string]$_) -match $guidPattern } | ForEach-Object { [string]$_ })
            if ($ids.Count -eq 0) { throw 'no valid update ids' }
            Invoke-Download $session $ids
        }
        'install' {
            if (([string]$req.id) -notmatch $guidPattern) { throw 'invalid update id' }
            Invoke-Install $session ([string]$req.id)
        }
        'status' {
            if (([string]$req.id) -notmatch $guidPattern) { throw 'invalid update id' }
            Invoke-Status $session ([string]$req.id)
        }
        default { throw 'unknown op' }
    }
} catch {
    $ex = $_.Exception
    while ($ex.InnerException) { $ex = $ex.InnerException }
    $h = ''
    if ($ex.HResult) { $h = Hex $ex.HResult }
    Emit @{ type = 'error'; message = [string]$ex.Message; hresult = $h }
}
