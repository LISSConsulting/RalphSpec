param(
    [Parameter(Position = 0)]
    [string]$Task = "help",

    [Parameter(Position = 1)]
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Dist = Join-Path $Root "dist"
$Repo = "LISSConsulting/RalphSpec"
$Binary = "ralph"
$Targets = @(
    @{ GOOS = "darwin"; GOARCH = "arm64"; Ext = "" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "linux"; GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" }
)

$term = if ($env:TERM) { $env:TERM } else { "" }
$hasRichTerminal = $env:WT_SESSION -or $env:TERM_PROGRAM -or $env:COLORTERM -or ($term -match "xterm|screen|tmux|rxvt|alacritty|wezterm|kitty")
$script:Fancy = (-not $env:NO_COLOR) -and ($term -ne "dumb") -and $hasRichTerminal -and (-not [Console]::IsOutputRedirected)

function Paint($Text, $Color) {
    if (-not $script:Fancy) { return $Text }
    $codes = @{
        Dim = "`e[2m"; Bold = "`e[1m"; Red = "`e[31m"; Green = "`e[32m"; Yellow = "`e[33m"
        Blue = "`e[34m"; Magenta = "`e[35m"; Cyan = "`e[36m"; Pink = "`e[38;5;205m"; Lime = "`e[38;5;154m"
    }
    return "$($codes[$Color])$Text`e[0m"
}

function Glyph($Fancy, $Plain) {
    if ($script:Fancy) { return $Fancy }
    return $Plain
}

function Banner($Title, $Sub = "") {
    $crown = Glyph "👑" "::"
    $line = Glyph "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" "------------------------------------------------------------"
    Write-Host ""
    Write-Host (Paint "$crown  $Title" "Pink")
    if ($Sub) { Write-Host (Paint "   $Sub" "Dim") }
    Write-Host (Paint $line "Magenta")
}

function Info($Text) { Write-Host "$(Paint (Glyph '◆' '>') 'Cyan') $Text" }
function Good($Text) { Write-Host "$(Paint (Glyph '✓' 'ok') 'Green') $Text" }
function Warn($Text) { Write-Host "$(Paint (Glyph '⚠' '!!') 'Yellow') $Text" }
function Bad($Text) { Write-Host "$(Paint (Glyph '✗' 'xx') 'Red') $Text" }

function Require-Cmd($Name, $Hint = "") {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        $message = "Missing required command: $Name"
        if ($Hint) { $message = "$message ($Hint)" }
        throw $message
    }
    Good "$Name available"
}

function Invoke-Step($Name, [scriptblock]$Action) {
    $start = Get-Date
    Info $Name
    try {
        & $Action
        $elapsed = ((Get-Date) - $start).TotalSeconds
        Good "$Name completed in $([math]::Round($elapsed, 1))s"
    } catch {
        Bad "$Name failed"
        throw
    }
}

function Current-Version($Fallback = "dev") {
    try {
        $v = ((& git describe --tags --always --dirty 2>$null) -join "`n").Trim()
        if ($v) { return $v }
    } catch {}
    return $Fallback
}

function Assert-Version($Value) {
    if (-not $Value) { throw "Version is required, for example: just release v0.2.0" }
    if ($Value -notmatch '^v\d+\.\d+\.\d+([-.][0-9A-Za-z.-]+)?$') {
        throw "Version must look like vMAJOR.MINOR.PATCH, got '$Value'"
    }
}

function Next-Version {
    $tags = & git tag --list "v[0-9]*.[0-9]*.[0-9]*"
    $latest = $tags |
        ForEach-Object {
            if ($_ -match '^v(\d+)\.(\d+)\.(\d+)$') {
                [pscustomobject]@{
                    Tag = $_
                    Major = [int]$Matches[1]
                    Minor = [int]$Matches[2]
                    Patch = [int]$Matches[3]
                }
            }
        } |
        Sort-Object Major, Minor, Patch -Descending |
        Select-Object -First 1

    if (-not $latest) { return "v0.1.0" }
    return "v$($latest.Major).$($latest.Minor).$($latest.Patch + 1)"
}

function Resolve-ReleaseVersion($Value) {
    if ($Value) {
        Assert-Version $Value
        return $Value
    }
    $next = Next-Version
    Info "No version supplied; next patch release is $next"
    return $next
}

function Assert-CleanTree {
    if ($env:ALLOW_DIRTY -eq "1") {
        Warn "ALLOW_DIRTY=1 set; skipping clean worktree guard"
        return
    }
    $dirty = ((& git status --porcelain) -join "`n").Trim()
    if ($dirty) { throw "Worktree has uncommitted changes. Commit/stash them or set ALLOW_DIRTY=1." }
    Good "worktree clean"
}

function Build-One($GOOS, $GOARCH, $Ext, $OutDir, $VersionValue) {
    $name = "$Binary-$GOOS-$GOARCH$Ext"
    $out = Join-Path $OutDir $name
    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH
    try {
        $env:GOOS = $GOOS
        $env:GOARCH = $GOARCH
        & go build -trimpath -ldflags="-s -w -X main.version=$VersionValue" -o $out ./cmd/ralph
    } finally {
        $env:GOOS = $oldGOOS
        $env:GOARCH = $oldGOARCH
    }
    $hash = (Get-FileHash -Algorithm SHA256 $out).Hash.ToLowerInvariant()
    $size = [math]::Round((Get-Item $out).Length / 1MB, 2)
    Good "$name  $size MiB  sha256:$($hash.Substring(0, 12))..."
}

function Build-Dist($VersionValue) {
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    foreach ($target in $Targets) {
        Build-One $target.GOOS $target.GOARCH $target.Ext $Dist $VersionValue
    }
}

function Publish-GitHubRelease($VersionValue) {
    Require-Cmd gh "https://cli.github.com"
    $existing = ((& gh release list --repo $Repo --limit 1000 --json tagName --jq ".[] | select(.tagName == `"$VersionValue`") | .tagName") -join "`n").Trim()
    $files = Get-ChildItem -Path $Dist -File -Filter "$Binary-*" | ForEach-Object { $_.FullName }
    if (-not $files) { throw "No dist assets found. Run just dist first." }

    if ($existing) {
        Warn "GitHub release $VersionValue exists; uploading assets with --clobber"
        & gh release upload $VersionValue @files --repo $Repo --clobber
    } else {
        & gh release create $VersionValue @files --repo $Repo --title "RalphSpec $VersionValue" --notes "Release $VersionValue"
    }
    Good "GitHub release assets published"
}

function Publish-Scoop($VersionValue) {
    Require-Cmd git
    Assert-Version $VersionValue

    $bucketRepo = if ($env:SCOOP_BUCKET_REPO) { $env:SCOOP_BUCKET_REPO } else { "https://github.com/LISSConsulting/scoop-bucket.git" }
    $manifestName = if ($env:SCOOP_MANIFEST) { $env:SCOOP_MANIFEST } else { "ralph.json" }
    $bucketDir = if ($env:SCOOP_BUCKET_DIR) { $env:SCOOP_BUCKET_DIR } else { Join-Path $env:TEMP "lisstech-scoop-bucket" }

    if (Test-Path $bucketDir) {
        Invoke-Step "refresh Scoop bucket" { & git -C $bucketDir pull --ff-only }
    } else {
        Invoke-Step "clone Scoop bucket" { & git clone $bucketRepo $bucketDir }
    }

    $asset = Join-Path $Dist "$Binary-windows-amd64.exe"
    if (-not (Test-Path $asset)) { throw "Missing Windows asset: $asset" }
    $hash = (Get-FileHash -Algorithm SHA256 $asset).Hash.ToLowerInvariant()
    $plainVersion = $VersionValue.TrimStart("v")
    $url = "https://github.com/$Repo/releases/download/$VersionValue/$Binary-windows-amd64.exe"
    $manifestPath = Join-Path $bucketDir "bucket/$manifestName"
    New-Item -ItemType Directory -Force -Path (Split-Path $manifestPath) | Out-Null

    $manifest = [ordered]@{
        version = $plainVersion
        description = "Spec-driven AI coding loop CLI with a Regent supervisor"
        homepage = "https://github.com/$Repo"
        license = "MIT"
        architecture = [ordered]@{
            "64bit" = [ordered]@{
                url = $url
                hash = $hash
            }
        }
        bin = "ralph.exe"
        checkver = [ordered]@{ github = "https://github.com/$Repo" }
        autoupdate = [ordered]@{
            architecture = [ordered]@{
                "64bit" = [ordered]@{
                    url = "https://github.com/$Repo/releases/download/v`$version/$Binary-windows-amd64.exe"
                }
            }
        }
    }

    $manifest | ConvertTo-Json -Depth 8 | Set-Content -Path $manifestPath -Encoding utf8
    Invoke-Step "commit Scoop manifest" {
        & git -C $bucketDir add "bucket/$manifestName"
        $changed = ((& git -C $bucketDir status --porcelain) -join "`n").Trim()
        if (-not $changed) {
            Warn "Scoop manifest already current"
            return
        }
        & git -C $bucketDir commit -m "ralph: update to $VersionValue"
    }
    Invoke-Step "push Scoop bucket" { & git -C $bucketDir push }
    Good "LISSTech Scoop bucket published: $manifestName $plainVersion"
}

function Show-Help {
    Banner "RalphSpec Command Deck" "Color and emoji automatically fall back for plain terminals."
    $rows = @(
        @("doctor", "toolchain + release prereq check"),
        @("deps", "download Go modules"),
        @("fmt", "gofmt all Go sources"),
        @("vet", "go vet ./..."),
        @("lint", "golangci-lint run, when installed"),
        @("test", "race tests with coverage"),
        @("coverage", "coverage.out + coverage.html"),
        @("build", "local ralph binary"),
        @("dist", "cross-compiled assets in dist/"),
        @("ci", "deps + fmt + vet + test + lint + dist"),
        @("snapshot", "local dist build using git describe"),
        @("release [vX.Y.Z]", "GitHub release + LISSTech Scoop bucket"),
        @("scoop [vX.Y.Z]", "publish only the Scoop manifest")
    )
    foreach ($row in $rows) {
        Write-Host "  $(Paint ($row[0].PadRight(16)) 'Lime') $(Paint $row[1] 'Dim')"
    }
    Write-Host ""
    Info "Release auto-bumps the latest vMAJOR.MINOR.PATCH tag by one patch."
    Info "Release knobs: SCOOP_BUCKET_REPO, SCOOP_BUCKET_DIR, SCOOP_MANIFEST, ALLOW_DIRTY=1"
}

Push-Location $Root
try {
    switch ($Task) {
        "help" { Show-Help }
        "doctor" {
            Banner "Doctor" "Checking tools before the kingdom rides."
            Require-Cmd git
            Require-Cmd go
            Require-Cmd just
            Require-Cmd gh "needed for release"
            if (Get-Command golangci-lint -ErrorAction SilentlyContinue) { Good "golangci-lint available" } else { Warn "golangci-lint missing; lint recipe will be skipped" }
            Info "Go: $((& go version) -join ' ')"
            Info "Version: $(Current-Version)"
        }
        "deps" { Banner "Dependencies"; Invoke-Step "go mod download" { & go mod download } }
        "fmt" { Banner "Format"; Invoke-Step "gofmt" { & gofmt -w (Get-ChildItem -Recurse -Filter *.go -Path cmd,internal | ForEach-Object FullName) } }
        "vet" { Banner "Vet"; Invoke-Step "go vet ./..." { & go vet ./... } }
        "lint" {
            Banner "Lint"
            if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
                Invoke-Step "golangci-lint run" { & golangci-lint run }
            } else {
                Warn "golangci-lint not installed; skipping"
            }
        }
        "test" { Banner "Tests"; Invoke-Step "go test -race -coverprofile=coverage.out ./..." { & go test -race -coverprofile=coverage.out ./... } }
        "coverage" {
            Banner "Coverage"
            Invoke-Step "write coverage.out" { & go test -race -coverprofile=coverage.out ./... }
            Invoke-Step "write coverage.html" { & go tool cover -html=coverage.out -o coverage.html }
        }
        "build" { Banner "Build"; Invoke-Step "go build ./cmd/ralph" { & go build -trimpath -ldflags="-s -w -X main.version=$(Current-Version)" -o $Binary ./cmd/ralph } }
        "dist" { Banner "Dist" "Cross-compiling release assets."; Invoke-Step "build release matrix" { Build-Dist (Current-Version) } }
        "ci" {
            Banner "Local CI" "Same spirit as GitHub Actions, louder outfit."
            Invoke-Step "go mod download" { & go mod download }
            Invoke-Step "gofmt" { & gofmt -w (Get-ChildItem -Recurse -Filter *.go -Path cmd,internal | ForEach-Object FullName) }
            Invoke-Step "go vet ./..." { & go vet ./... }
            Invoke-Step "go test -race -coverprofile=coverage.out ./..." { & go test -race -coverprofile=coverage.out ./... }
            if (Get-Command golangci-lint -ErrorAction SilentlyContinue) { Invoke-Step "golangci-lint run" { & golangci-lint run } } else { Warn "golangci-lint not installed; skipping" }
            Invoke-Step "build release matrix" { Build-Dist (Current-Version) }
        }
        "clean" {
            Banner "Clean"
            Invoke-Step "remove generated artifacts" {
                Remove-Item -Force -Recurse -ErrorAction SilentlyContinue $Dist, "coverage.out", "coverage.html", "ralph", "ralph.exe"
            }
        }
        "snapshot" { Banner "Snapshot"; Invoke-Step "build snapshot assets" { Build-Dist (Current-Version) } }
        "release" {
            $Version = Resolve-ReleaseVersion $Version
            Banner "Release $Version" "Tests, assets, GitHub release, and LISSTech Scoop bucket."
            Assert-CleanTree
            Invoke-Step "go mod download" { & go mod download }
            Invoke-Step "go vet ./..." { & go vet ./... }
            Invoke-Step "go test -race -cover ./..." { & go test -race -cover ./... }
            if (Get-Command golangci-lint -ErrorAction SilentlyContinue) { Invoke-Step "golangci-lint run" { & golangci-lint run } } else { Warn "golangci-lint not installed; skipping" }
            Invoke-Step "build release assets" { Build-Dist $Version }
            Invoke-Step "publish GitHub release" { Publish-GitHubRelease $Version }
            Publish-Scoop $Version
        }
        "scoop" {
            $Version = Resolve-ReleaseVersion $Version
            Banner "Scoop $Version"
            Publish-Scoop $Version
        }
        default { throw "Unknown task '$Task'. Run just for the command deck." }
    }
} finally {
    Pop-Location
}
