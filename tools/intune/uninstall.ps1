if ($env:PROCESSOR_ARCHITEW6432) {
    & "$env:SystemRoot\Sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File $PSCommandPath
    exit $LASTEXITCODE
}

$ErrorActionPreference = "Stop"
$Server = "https://patchmon.example.com"

$Server = $Server.TrimEnd("/")
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$remover = Join-Path $env:TEMP "patchmon-remove.ps1"
Invoke-WebRequest -Uri "$Server/api/v1/hosts/remove?os=windows" -UseBasicParsing -OutFile $remover
& "$PSHOME\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File $remover -RemoveAll -Force | Out-Host
$code = $LASTEXITCODE
Remove-Item -Path $remover -Force -ErrorAction SilentlyContinue
exit $code
