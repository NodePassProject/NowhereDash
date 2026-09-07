[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet("install", "update", "uninstall", "status")]
    [string]$Action = "install",

    [Parameter(Position = 1)]
    [ValidateSet("dash", "nowhere", "openctrl", "all")]
    [string]$Target = "dash",

    [switch]$Yes,
    [switch]$Purge,
    [switch]$NoFirewall,
    [string]$GitHubProxy = $env:NOWHEREDASH_GITHUB_PROXY,

    [ValidateRange(1, 65535)]
    [int]$DashPort = 4000,
    [ValidateSet("stable", "beta")]
    [string]$DashChannel = "stable",
    [string]$DashVersion = "",
    [string]$DashCert = "",
    [string]$DashKey = "",

    [string]$OpenCtrlListen = "0.0.0.0",
    [string]$OpenCtrlPublicHost = "",
    [ValidateRange(1, 65535)]
    [int]$OpenCtrlPort = 10101,
    [string]$OpenCtrlPrefix = "api",
    [ValidateSet(0, 1, 2)]
    [int]$OpenCtrlTls = 1,
    [string]$OpenCtrlCert = "",
    [string]$OpenCtrlKey = "",
    [string]$OpenCtrlVersion = "",
    [string]$NowhereVersion = "",
    [string]$RegisterUrl = "",
    [string]$RegisterToken = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$DashRepo = if ($env:NOWHEREDASH_REPO) { $env:NOWHEREDASH_REPO } else { "NodePassProject/NowhereDash" }
$OpenCtrlRepo = if ($env:OPENCTRL_REPO) { $env:OPENCTRL_REPO } else { "NodePassProject/OpenCtrl" }
$NowhereRepo = if ($env:NOWHERE_REPO) { $env:NOWHERE_REPO } else { "NodePassProject/Nowhere" }

$DashInstallDir = Join-Path $env:ProgramData "NowhereDash"
$DashBinDir = Join-Path $DashInstallDir "bin"
$DashBinary = Join-Path $DashBinDir "nowheredash.exe"
$DashConfig = Join-Path $DashInstallDir "install.json"
$DashRunner = Join-Path $DashInstallDir "run.ps1"
$DashTask = "NowhereDash"

$NodeInstallDir = Join-Path $env:ProgramData "OpenCtrl"
$NodeBinDir = Join-Path $NodeInstallDir "bin"
$NodeStateDir = Join-Path $NodeBinDir "gob"
$OpenCtrlBinary = Join-Path $NodeBinDir "openctrl.exe"
$NowhereBinary = Join-Path $NodeBinDir "nowhere.exe"
$NodeConfigDir = Join-Path $env:ProgramData "OpenCtrlConfig"
$NodeConfig = Join-Path $NodeConfigDir "install.json"
$NodeEndpoint = Join-Path $NodeConfigDir "endpoint.json"
$NodeRunner = Join-Path $NodeInstallDir "run.ps1"
$NodeTask = "OpenCtrl"

$script:WorkDir = ""
$script:InstallerParameters = $PSBoundParameters

function Write-Info([string]$Message) {
    Write-Host "[INFO] $Message" -ForegroundColor Cyan
}

function Write-Success([string]$Message) {
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "Run this installer from an Administrator PowerShell session."
    }
}

function Assert-WindowsX64 {
    if (-not [Environment]::Is64BitOperatingSystem) {
        throw "The Nowhere Windows release is currently available only for x86_64."
    }
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -notin @("AMD64", "x86")) {
        throw "Unsupported Windows architecture: $arch. Nowhere currently provides only x86_64."
    }
}

function Assert-SafeValue([string]$Name, [string]$Value) {
    if ($Value.Contains("`r") -or $Value.Contains("`n")) {
        throw "$Name contains an invalid newline."
    }
}

function Assert-Version([string]$Name, [string]$Value) {
    if ($Value -and $Value -notmatch '^v?\d+(\.\d+){1,3}([.-][A-Za-z0-9._-]+)?$') {
        throw "Invalid $Name version: $Value"
    }
}

function Get-ProxiedUrl([string]$Url) {
    if ([string]::IsNullOrWhiteSpace($GitHubProxy)) {
        return $Url
    }
    return "$($GitHubProxy.TrimEnd('/'))/$Url"
}

function Get-GitHubHeaders {
    $headers = @{
        Accept = "application/vnd.github+json"
        "User-Agent" = "NowhereDash-Windows-Installer"
    }
    if ($env:GITHUB_TOKEN) {
        $headers.Authorization = "Bearer $($env:GITHUB_TOKEN)"
    }
    return $headers
}

function Get-Release([string]$Repo, [string]$RequestedVersion, [string]$Channel) {
    if ($RequestedVersion) {
        $tag = if ($RequestedVersion.StartsWith("v")) { $RequestedVersion } else { "v$RequestedVersion" }
        $url = "https://api.github.com/repos/$Repo/releases/tags/$tag"
    }
    elseif ($Channel -eq "beta") {
        $listUrl = Get-ProxiedUrl "https://api.github.com/repos/$Repo/releases?per_page=30"
        $releases = Invoke-RestMethod -UseBasicParsing -Headers (Get-GitHubHeaders) -Uri $listUrl
        $release = $releases | Where-Object { $_.prerelease -and -not $_.draft } | Select-Object -First 1
        if (-not $release) {
            throw "$Repo does not have an available prerelease."
        }
        return $release
    }
    else {
        $url = "https://api.github.com/repos/$Repo/releases/latest"
    }

    return Invoke-RestMethod -UseBasicParsing -Headers (Get-GitHubHeaders) -Uri (Get-ProxiedUrl $url)
}

function Test-ArchiveEntry([string]$Name, [string]$Root) {
    if ([string]::IsNullOrWhiteSpace($Name) -or [IO.Path]::IsPathRooted($Name)) {
        return $false
    }
    $normalizedRoot = [IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    $candidate = [IO.Path]::GetFullPath((Join-Path $Root $Name))
    return $candidate.StartsWith($normalizedRoot, [StringComparison]::OrdinalIgnoreCase)
}

function Expand-SafeArchive([string]$Archive, [string]$Destination, [string]$AssetName) {
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    if ($AssetName.EndsWith(".zip", [StringComparison]::OrdinalIgnoreCase)) {
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
        try {
            foreach ($entry in $zip.Entries) {
                if (-not (Test-ArchiveEntry $entry.FullName $Destination)) {
                    throw "Release archive contains an unsafe path: $($entry.FullName)"
                }
            }
        }
        finally {
            $zip.Dispose()
        }
        [IO.Compression.ZipFile]::ExtractToDirectory($Archive, $Destination)
        return
    }

    $entries = & tar.exe -tzf $Archive
    if ($LASTEXITCODE -ne 0) {
        throw "Cannot read release archive: $AssetName"
    }
    foreach ($entry in $entries) {
        if (-not (Test-ArchiveEntry $entry $Destination)) {
            throw "Release archive contains an unsafe path: $entry"
        }
    }
    & tar.exe -xzf $Archive -C $Destination
    if ($LASTEXITCODE -ne 0) {
        throw "Cannot extract release archive: $AssetName"
    }
}

function Get-ReleaseBinary(
    [string]$Label,
    [string]$Repo,
    [string]$RequestedVersion,
    [string]$Channel,
    [string]$AssetPattern,
    [string]$BinaryName
) {
    $release = Get-Release $Repo $RequestedVersion $Channel
    $asset = $release.assets | Where-Object { $_.name -match $AssetPattern } | Select-Object -First 1
    if (-not $asset) {
        throw "No compatible asset was found in $Repo $($release.tag_name)."
    }

    $archive = Join-Path $script:WorkDir $asset.name
    Write-Info "Downloading $Repo $($release.tag_name): $($asset.name)"
    Invoke-WebRequest -UseBasicParsing -Headers (Get-GitHubHeaders) -Uri (Get-ProxiedUrl $asset.browser_download_url) -OutFile $archive
    if ((Get-Item $archive).Length -eq 0) {
        throw "Downloaded release archive is empty: $($asset.name)"
    }

    $digestProperty = $asset.PSObject.Properties["digest"]
    $digest = if ($digestProperty) { [string]$digestProperty.Value } else { "" }
    if ($digest -and $digest.StartsWith("sha256:")) {
        $expected = $digest.Substring(7).ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "SHA-256 verification failed: $($asset.name)"
        }
        Write-Success "SHA-256 verified: $($asset.name)"
    }
    else {
        Write-Warning "$Repo $($release.tag_name) does not expose a digest for $($asset.name)."
    }

    $extractDir = Join-Path $script:WorkDir "$Label-extract"
    Expand-SafeArchive $archive $extractDir $asset.name
    $binary = Get-ChildItem -Path $extractDir -Recurse -File |
        Where-Object { $_.Name -ieq $BinaryName } |
        Select-Object -First 1
    if (-not $binary) {
        throw "The archive does not contain $BinaryName."
    }

    $staged = Join-Path $script:WorkDir "$Label-$BinaryName"
    Copy-Item -Force $binary.FullName $staged
    return [pscustomobject]@{
        Path = $staged
        Version = [string]$release.tag_name
        Asset = [string]$asset.name
    }
}

function Install-BinaryAtomic([string]$Source, [string]$Destination) {
    New-Item -ItemType Directory -Force -Path (Split-Path $Destination) | Out-Null
    if (Test-Path $Destination) {
        Copy-Item -Force $Destination "$Destination.previous"
    }
    Copy-Item -Force $Source "$Destination.new"
    Move-Item -Force "$Destination.new" $Destination
}

function Copy-FileIfDifferent([string]$Source, [string]$Destination) {
    $sourcePath = [IO.Path]::GetFullPath($Source)
    $destinationPath = [IO.Path]::GetFullPath($Destination)
    if (-not $sourcePath.Equals($destinationPath, [StringComparison]::OrdinalIgnoreCase)) {
        Copy-Item -Force $Source $Destination
    }
}

function Restore-Binary([string]$Destination) {
    if (Test-Path "$Destination.previous") {
        Write-Warning "Restoring previous binary: $Destination"
        Move-Item -Force "$Destination.previous" $Destination
    }
}

function Protect-AdminFile([string]$Path) {
    & icacls.exe $Path /inheritance:r /grant:r "*S-1-5-18:(F)" "*S-1-5-32-544:(F)" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to restrict permissions on $Path."
    }
}

function Protect-AdminTree([string]$Path) {
    & icacls.exe $Path /inheritance:r /grant:r `
        "*S-1-5-18:(OI)(CI)(F)" "*S-1-5-32-544:(OI)(CI)(F)" /T | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to restrict permissions on $Path."
    }
}

function Register-StartupTask([string]$TaskName, [string]$Runner) {
    $escapedRunner = $Runner.Replace('"', '""')
    $taskAction = New-ScheduledTaskAction -Execute "powershell.exe" `
        -Argument "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$escapedRunner`"" `
        -WorkingDirectory (Split-Path $Runner)
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $settings = New-ScheduledTaskSettingsSet `
        -RestartCount 999 `
        -RestartInterval (New-TimeSpan -Minutes 1) `
        -ExecutionTimeLimit ([TimeSpan]::Zero) `
        -MultipleInstances IgnoreNew `
        -StartWhenAvailable `
        -AllowStartIfOnBatteries `
        -DontStopIfGoingOnBatteries
    Register-ScheduledTask -TaskName $TaskName -Action $taskAction -Trigger $trigger `
        -Settings $settings -User "SYSTEM" -RunLevel Highest -Force | Out-Null
    Start-ScheduledTask -TaskName $TaskName
}

function Stop-StartupTask([string]$TaskName) {
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
        for ($i = 0; $i -lt 20; $i++) {
            $state = (Get-ScheduledTask -TaskName $TaskName).State
            if ($state -ne "Running") { break }
            Start-Sleep -Milliseconds 250
        }
    }
}

function Wait-TaskRunning([string]$TaskName) {
    for ($i = 0; $i -lt 20; $i++) {
        if ((Get-ScheduledTask -TaskName $TaskName).State -eq "Running") {
            return $true
        }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Add-FirewallPort([string]$Name, [int]$Port) {
    if ($NoFirewall) { return }
    Remove-NetFirewallRule -DisplayName $Name -ErrorAction SilentlyContinue
    New-NetFirewallRule -DisplayName $Name -Direction Inbound -Action Allow `
        -Protocol TCP -LocalPort $Port | Out-Null
}

function Remove-FirewallPort([string]$Name) {
    Remove-NetFirewallRule -DisplayName $Name -ErrorAction SilentlyContinue
}

function Get-PublicHost {
    if ($OpenCtrlPublicHost) { return $OpenCtrlPublicHost }
    try {
        $detected = (Invoke-RestMethod -UseBasicParsing -TimeoutSec 5 -Uri "https://api.ipify.org").Trim()
        if ($detected) { return $detected }
    }
    catch {
        Write-Warning "Could not detect a public IP; using 127.0.0.1."
    }
    return "127.0.0.1"
}

function Format-UrlHost([string]$Value) {
    if ($Value.StartsWith("[") -and $Value.EndsWith("]")) { return $Value }
    if ($Value.Contains(":")) { return "[$Value]" }
    return $Value
}

function ConvertTo-ImportUri([string]$ApiUrl, [string]$ApiKey) {
    $url64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($ApiUrl))
    $key64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($ApiKey))
    return "np://master?url=$url64&key=$key64"
}

function Get-NodeApiKey {
    $stateFile = Join-Path $NodeStateDir "openctrl.gob"
    for ($i = 0; $i -lt 20; $i++) {
        if (Test-Path $stateFile) {
            $bytes = [IO.File]::ReadAllBytes($stateFile)
            $text = [Text.Encoding]::ASCII.GetString($bytes)
            $match = [regex]::Match($text, '[0-9a-f]{32}')
            if ($match.Success) { return $match.Value }
        }
        Start-Sleep -Seconds 1
    }
    throw "Could not read the OpenCtrl API key from its state file."
}

function Test-NodeApi([string]$ApiKey, [System.Collections.IDictionary]$Config) {
    $scheme = if ([int]$Config.Tls -eq 0) { "http" } else { "https" }
    $listen = [string]$Config.ListenHost
    if ($listen -eq "0.0.0.0" -or $listen -eq "") { $listen = "127.0.0.1" }
    elseif ($listen -eq "::") { $listen = "[::1]" }
    else { $listen = Format-UrlHost $listen }
    $uri = "${scheme}://${listen}:$($Config.Port)/$($Config.Prefix)/v2/info"
    $oldCallback = [Net.ServicePointManager]::ServerCertificateValidationCallback
    [Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
    try {
        for ($i = 0; $i -lt 20; $i++) {
            try {
                Invoke-WebRequest -UseBasicParsing -TimeoutSec 3 -Headers @{ "X-API-Key" = $ApiKey } -Uri $uri | Out-Null
                return $true
            }
            catch {
                Start-Sleep -Seconds 1
            }
        }
        return $false
    }
    finally {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = $oldCallback
    }
}

function Write-NodeRunner {
    $content = @'
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$configPath = Join-Path $PSScriptRoot "..\OpenCtrlConfig\install.json"
$config = Get-Content -Raw $configPath | ConvertFrom-Json
Set-Location $config.InstallDir
& $config.OpenCtrlBinary $config.MasterUrl *>> $config.LogFile
exit $LASTEXITCODE
'@
    Set-Content -Encoding UTF8 -Path $NodeRunner -Value $content
}

function Install-Node {
    Assert-WindowsX64
    Assert-Version "OpenCtrl" $OpenCtrlVersion
    Assert-Version "Nowhere" $NowhereVersion
    Assert-SafeValue "OpenCtrlListen" $OpenCtrlListen
    Assert-SafeValue "OpenCtrlPublicHost" $OpenCtrlPublicHost
    Assert-SafeValue "OpenCtrlPrefix" $OpenCtrlPrefix
    if ($OpenCtrlListen -notmatch '^[A-Za-z0-9._:\[\]-]+$') { throw "Invalid OpenCtrl listen host." }
    if ($OpenCtrlPrefix -notmatch '^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$') { throw "Invalid OpenCtrl API prefix." }
    if ($OpenCtrlTls -eq 2) {
        if (-not (Test-Path -PathType Leaf $OpenCtrlCert)) { throw "OpenCtrl certificate does not exist: $OpenCtrlCert" }
        if (-not (Test-Path -PathType Leaf $OpenCtrlKey)) { throw "OpenCtrl private key does not exist: $OpenCtrlKey" }
    }
    if (($RegisterUrl -and -not $RegisterToken) -or ($RegisterToken -and -not $RegisterUrl)) {
        throw "RegisterUrl and RegisterToken must be supplied together."
    }

    $openCtrl = Get-ReleaseBinary "openctrl" $OpenCtrlRepo $OpenCtrlVersion "stable" `
        '^openctrl_.*_windows_amd64\.tar\.gz$' "openctrl.exe"
    $nowhere = Get-ReleaseBinary "nowhere" $NowhereRepo $NowhereVersion "stable" `
        '^nowhere-x86_64-pc-windows-msvc\.zip$' "nowhere.exe"

    Stop-StartupTask $NodeTask
    New-Item -ItemType Directory -Force -Path $NodeBinDir, $NodeStateDir, $NodeConfigDir, (Join-Path $NodeInstallDir "logs"), (Join-Path $NodeConfigDir "certs") | Out-Null
    Protect-AdminTree $NodeInstallDir
    Protect-AdminTree $NodeConfigDir

    $certPath = ""
    $keyPath = ""
    if ($OpenCtrlTls -eq 2) {
        $certPath = Join-Path $NodeConfigDir "certs\fullchain.pem"
        $keyPath = Join-Path $NodeConfigDir "certs\privkey.pem"
        Copy-FileIfDifferent $OpenCtrlCert $certPath
        Copy-FileIfDifferent $OpenCtrlKey $keyPath
    }

    $runtimeParam = [Uri]::EscapeDataString($NowhereBinary)
    $listen = Format-UrlHost $OpenCtrlListen
    $masterUrl = "master://${listen}:${OpenCtrlPort}/${OpenCtrlPrefix}?tls=${OpenCtrlTls}&bin=${runtimeParam}"
    if ($OpenCtrlTls -eq 2) {
        $masterUrl += "&crt=$([Uri]::EscapeDataString($certPath))&key=$([Uri]::EscapeDataString($keyPath))"
    }
    $publicHost = Get-PublicHost
    $config = [ordered]@{
        InstallDir = $NodeInstallDir
        OpenCtrlBinary = $OpenCtrlBinary
        NowhereBinary = $NowhereBinary
        MasterUrl = $masterUrl
        ListenHost = $OpenCtrlListen
        PublicHost = $publicHost
        Port = $OpenCtrlPort
        Prefix = $OpenCtrlPrefix
        Tls = $OpenCtrlTls
        CertPath = $certPath
        KeyPath = $keyPath
        OpenCtrlVersion = $openCtrl.Version
        NowhereVersion = $nowhere.Version
        GitHubProxy = $GitHubProxy
        LogFile = (Join-Path $NodeInstallDir "logs\openctrl.log")
    }
    $config | ConvertTo-Json | Set-Content -Encoding UTF8 $NodeConfig
    Protect-AdminFile $NodeConfig
    Write-NodeRunner

    $hadOpenCtrl = Test-Path $OpenCtrlBinary
    $hadNowhere = Test-Path $NowhereBinary
    Install-BinaryAtomic $openCtrl.Path $OpenCtrlBinary
    Install-BinaryAtomic $nowhere.Path $NowhereBinary
    Register-StartupTask $NodeTask $NodeRunner

    if (-not (Wait-TaskRunning $NodeTask)) {
        if ($hadOpenCtrl) { Restore-Binary $OpenCtrlBinary }
        if ($hadNowhere) { Restore-Binary $NowhereBinary }
        throw "OpenCtrl failed to start. See $($config.LogFile)."
    }

    try {
        $apiKey = Get-NodeApiKey
        if (-not (Test-NodeApi $apiKey $config)) {
            throw "OpenCtrl API health check failed."
        }
    }
    catch {
        Stop-StartupTask $NodeTask
        if ($hadOpenCtrl) { Restore-Binary $OpenCtrlBinary }
        if ($hadNowhere) { Restore-Binary $NowhereBinary }
        Start-ScheduledTask -TaskName $NodeTask -ErrorAction SilentlyContinue
        throw
    }

    $scheme = if ($OpenCtrlTls -eq 0) { "http" } else { "https" }
    $base = "${scheme}://$(Format-UrlHost $publicHost):${OpenCtrlPort}"
    $apiUrl = "$base/${OpenCtrlPrefix}/v2"
    $importUri = ConvertTo-ImportUri $apiUrl $apiKey
    $endpoint = [ordered]@{ ApiUrl = $apiUrl; ApiKey = $apiKey; ImportUri = $importUri }
    $endpoint | ConvertTo-Json | Set-Content -Encoding UTF8 $NodeEndpoint
    Protect-AdminFile $NodeEndpoint
    Add-FirewallPort "OpenCtrl TCP $OpenCtrlPort" $OpenCtrlPort

    if ($RegisterUrl) {
        $payload = @{ token = $RegisterToken; apiUrl = $apiUrl; apiKey = $apiKey; hostname = $publicHost } | ConvertTo-Json
        Invoke-RestMethod -UseBasicParsing -Method Post -ContentType "application/json" -Body $payload -Uri $RegisterUrl | Out-Null
        Write-Success "Node registered with NowhereDash."
    }

    Write-Success "OpenCtrl $($openCtrl.Version) and Nowhere $($nowhere.Version) are running."
    Write-Host "API URL: $apiUrl"
    Write-Host "API KEY: $apiKey"
    Write-Host "URI: $importUri"
}

function Write-DashRunner {
    $content = @'
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$config = Get-Content -Raw (Join-Path $PSScriptRoot "install.json") | ConvertFrom-Json
$env:PORT = [string]$config.Port
$env:DB_PATH = [string]$config.DbPath
$env:GITHUB_PROXY = [string]$config.GitHubProxy
if ($config.CertPath) { $env:TLS_CERT = [string]$config.CertPath }
if ($config.KeyPath) { $env:TLS_KEY = [string]$config.KeyPath }
Set-Location $config.InstallDir
& $config.Binary *>> $config.LogFile
exit $LASTEXITCODE
'@
    Set-Content -Encoding UTF8 -Path $DashRunner -Value $content
}

function Test-Dash([System.Collections.IDictionary]$Config) {
    $scheme = if ($Config.CertPath) { "https" } else { "http" }
    $uri = "${scheme}://127.0.0.1:$($Config.Port)/"
    $oldCallback = [Net.ServicePointManager]::ServerCertificateValidationCallback
    [Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
    try {
        for ($i = 0; $i -lt 20; $i++) {
            try {
                Invoke-WebRequest -UseBasicParsing -TimeoutSec 3 -Uri $uri | Out-Null
                return $true
            }
            catch {
                Start-Sleep -Seconds 1
            }
        }
        return $false
    }
    finally {
        [Net.ServicePointManager]::ServerCertificateValidationCallback = $oldCallback
    }
}

function Install-Dash {
    Assert-WindowsX64
    Assert-Version "NowhereDash" $DashVersion
    if (($DashCert -and -not $DashKey) -or ($DashKey -and -not $DashCert)) {
        throw "DashCert and DashKey must be supplied together."
    }
    if ($DashCert -and -not (Test-Path -PathType Leaf $DashCert)) { throw "Dash certificate does not exist: $DashCert" }
    if ($DashKey -and -not (Test-Path -PathType Leaf $DashKey)) { throw "Dash private key does not exist: $DashKey" }

    $dash = Get-ReleaseBinary "dash" $DashRepo $DashVersion $DashChannel `
        '^nowheredash_windows_x86_64\.zip$' "nowheredash.exe"
    Stop-StartupTask $DashTask
    New-Item -ItemType Directory -Force -Path $DashBinDir, (Join-Path $DashInstallDir "db"), (Join-Path $DashInstallDir "logs"), (Join-Path $DashInstallDir "backups"), (Join-Path $DashInstallDir "certs") | Out-Null
    Protect-AdminTree $DashInstallDir
    $envFile = Join-Path $DashInstallDir ".env"
    if (-not (Test-Path $envFile)) { New-Item -ItemType File -Path $envFile | Out-Null }

    $certPath = ""
    $keyPath = ""
    if ($DashCert) {
        $certPath = Join-Path $DashInstallDir "certs\server.crt"
        $keyPath = Join-Path $DashInstallDir "certs\server.key"
        Copy-FileIfDifferent $DashCert $certPath
        Copy-FileIfDifferent $DashKey $keyPath
    }
    $config = [ordered]@{
        InstallDir = $DashInstallDir
        Binary = $DashBinary
        Version = $dash.Version
        Channel = $DashChannel
        Port = $DashPort
        DbPath = (Join-Path $DashInstallDir "db\database.db")
        CertPath = $certPath
        KeyPath = $keyPath
        GitHubProxy = $GitHubProxy
        LogFile = (Join-Path $DashInstallDir "logs\nowheredash.log")
    }
    $config | ConvertTo-Json | Set-Content -Encoding UTF8 $DashConfig
    Protect-AdminFile $DashConfig
    Write-DashRunner

    $hadOld = Test-Path $DashBinary
    Install-BinaryAtomic $dash.Path $DashBinary
    Register-StartupTask $DashTask $DashRunner
    if (-not (Wait-TaskRunning $DashTask) -or -not (Test-Dash $config)) {
        Stop-StartupTask $DashTask
        if ($hadOld) {
            Restore-Binary $DashBinary
            Start-ScheduledTask -TaskName $DashTask -ErrorAction SilentlyContinue
        }
        throw "NowhereDash startup check failed. See $($config.LogFile)."
    }
    Add-FirewallPort "NowhereDash TCP $DashPort" $DashPort
    $scheme = if ($certPath) { "https" } else { "http" }
    Write-Success "NowhereDash $($dash.Version) is running."
    Write-Host "URL: ${scheme}://localhost:${DashPort}"
    Write-Host "Data directory: $DashInstallDir"
}

function Uninstall-Node {
    Stop-StartupTask $NodeTask
    Unregister-ScheduledTask -TaskName $NodeTask -Confirm:$false -ErrorAction SilentlyContinue
    Remove-FirewallPort "OpenCtrl TCP $OpenCtrlPort"
    Remove-Item -Recurse -Force $NodeInstallDir, $NodeConfigDir -ErrorAction SilentlyContinue
    Write-Success "OpenCtrl, Nowhere, state, API key, and configuration were removed."
}

function Uninstall-Dash {
    Stop-StartupTask $DashTask
    Unregister-ScheduledTask -TaskName $DashTask -Confirm:$false -ErrorAction SilentlyContinue
    Remove-FirewallPort "NowhereDash TCP $DashPort"
    if ($Purge) {
        Remove-Item -Recurse -Force $DashInstallDir -ErrorAction SilentlyContinue
        Write-Success "NowhereDash and its data were removed."
    }
    else {
        Remove-Item -Force $DashBinary, "$DashBinary.previous", $DashRunner -ErrorAction SilentlyContinue
        Write-Success "NowhereDash was removed; data remains in $DashInstallDir."
    }
}

function Show-TaskStatus([string]$TaskName) {
    $task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if (-not $task) {
        Write-Warning "$TaskName is not installed."
        return
    }
    $info = Get-ScheduledTaskInfo -TaskName $TaskName
    [pscustomobject]@{
        Task = $TaskName
        State = $task.State
        LastRunTime = $info.LastRunTime
        LastTaskResult = $info.LastTaskResult
        NextRunTime = $info.NextRunTime
    } | Format-List
}

function Import-ExistingConfiguration {
    if (($Target -in @("nowhere", "all")) -and (Test-Path $NodeConfig)) {
        $saved = Get-Content -Raw $NodeConfig | ConvertFrom-Json
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlListen")) { $script:OpenCtrlListen = [string]$saved.ListenHost }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlPublicHost")) { $script:OpenCtrlPublicHost = [string]$saved.PublicHost }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlPort")) { $script:OpenCtrlPort = [int]$saved.Port }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlPrefix")) { $script:OpenCtrlPrefix = [string]$saved.Prefix }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlTls")) { $script:OpenCtrlTls = [int]$saved.Tls }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlCert")) { $script:OpenCtrlCert = [string]$saved.CertPath }
        if (-not $script:InstallerParameters.ContainsKey("OpenCtrlKey")) { $script:OpenCtrlKey = [string]$saved.KeyPath }
        if (-not $script:InstallerParameters.ContainsKey("GitHubProxy")) { $script:GitHubProxy = [string]$saved.GitHubProxy }
    }
    if (($Target -in @("dash", "all")) -and (Test-Path $DashConfig)) {
        $saved = Get-Content -Raw $DashConfig | ConvertFrom-Json
        if (-not $script:InstallerParameters.ContainsKey("DashPort")) { $script:DashPort = [int]$saved.Port }
        if (-not $script:InstallerParameters.ContainsKey("DashChannel")) { $script:DashChannel = [string]$saved.Channel }
        if (-not $script:InstallerParameters.ContainsKey("DashCert")) { $script:DashCert = [string]$saved.CertPath }
        if (-not $script:InstallerParameters.ContainsKey("DashKey")) { $script:DashKey = [string]$saved.KeyPath }
        if (-not $script:InstallerParameters.ContainsKey("GitHubProxy")) { $script:GitHubProxy = [string]$saved.GitHubProxy }
    }
}

try {
    if ($Target -eq "openctrl") { $Target = "nowhere" }
    Assert-SafeValue "GitHubProxy" $GitHubProxy

    if ($Action -eq "status") {
        if ($Target -in @("nowhere", "all")) { Show-TaskStatus $NodeTask }
        if ($Target -in @("dash", "all")) { Show-TaskStatus $DashTask }
        exit 0
    }

    Assert-Administrator
    Import-ExistingConfiguration
    Assert-SafeValue "GitHubProxy" $GitHubProxy
    if ($Action -eq "uninstall") {
        if (-not $Yes) {
            $answer = Read-Host "Uninstall $Target? [y/N]"
            if ($answer -notmatch '^[Yy]') { exit 0 }
        }
        if ($Target -in @("nowhere", "all")) { Uninstall-Node }
        if ($Target -in @("dash", "all")) { Uninstall-Dash }
        exit 0
    }

    if ($Target -eq "all" -and $DashPort -eq $OpenCtrlPort) {
        throw "NowhereDash and OpenCtrl cannot use the same port: $DashPort"
    }
    $script:WorkDir = Join-Path ([IO.Path]::GetTempPath()) ("nowheredash-install-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $script:WorkDir | Out-Null
    if ($Target -in @("nowhere", "all")) { Install-Node }
    if ($Target -in @("dash", "all")) { Install-Dash }
}
catch {
    Write-Error $_
    exit 1
}
finally {
    if ($script:WorkDir -and (Test-Path $script:WorkDir)) {
        Remove-Item -Recurse -Force $script:WorkDir -ErrorAction SilentlyContinue
    }
}
