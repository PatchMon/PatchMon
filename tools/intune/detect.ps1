$svc = Get-Service -Name PatchMonAgent -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") { "Installed" }
