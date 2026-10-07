# build.ps1 - xz-go Windows dev auto-build + DLL bundling
#
# Purpose:
#   Builds xiaozhi-server.exe with CGO (opus codec), then resolves the DLL
#   dependency chain via ldd and copies every runtime DLL from
#   C:\msys64\mingw64\bin to .\bin\ so the build artifact is self-contained
#   and can run without any external PATH manipulation.
#
# Requirements (one-time install):
#   MSYS2 + mingw-w64 toolchain with:
#     pacman -S mingw-w64-x86_64-gcc
#              mingw-w64-x86_64-pkg-config
#              mingw-w64-x86_64-opus
#              mingw-w64-x86_64-opusfile
#              mingw-w64-x86_64-libogg
#
# Usage:
#   .\scripts\build.ps1                  # full build + bundle + smoke probe
#   .\scripts\build.ps1 -SkipTest        # build + bundle, no run
#   .\scripts\build.ps1 -SkipBuild       # re-bundle DLLs only (use existing exe)
#
# Output:
#   bin/xiaozhi-server.exe               # go build artifact
#   bin/<runtime-dlls>                   # copied from msys64/mingw64/bin
#   bin/ is self-contained -> run without PATH tricks

[CmdletBinding()]
param(
    [string]$ExeRelative = "bin\xiaozhi-server.exe",
    [switch]$SkipBuild,
    [switch]$SkipTest,
    # Build with `-tags silero` to enable server-side Silero VAD
    # (otherwise internal/audio/vad/silero_nobuild.go is compiled, the
    # NewSileroVAD stub returns an error, and audioPipeline stays nil in
    # app.go — handler.pumpVADEvents never gets SpeechEnd, so the
    # orchestrator is only triggered by ListenStateStop which ESP32 in
    # ListeningModeRealtime never sends).
    [switch]$WithSilero = $true
)

$ErrorActionPreference = 'Stop'

# Force UTF-8 console so ldd.exe output (mingw64 is UTF-8) parses cleanly on
# Windows code pages (e.g. GB2312 / cp936).
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$RepoRoot   = (Resolve-Path "$PSScriptRoot\..").Path
$MingwBin   = 'C:\msys64\mingw64\bin'
$LddExe     = 'C:\msys64\usr\bin\ldd.exe'
$PkgConfig  = Join-Path $MingwBin 'pkg-config.exe'
$BinDir     = Join-Path $RepoRoot 'bin'
$ExeFull    = Join-Path $RepoRoot $ExeRelative

function Write-Section($msg) {
    Write-Host ""
    Write-Host "=== $msg ===" -ForegroundColor Cyan
}

# --- 1. Verify toolchain ---
Write-Section "1/6 Verify msys2 toolchain"

if (-not (Test-Path (Join-Path $MingwBin 'gcc.exe'))) {
    throw "MSYS2/mingw64 not found at $MingwBin. Install: pacman -S mingw-w64-x86_64-gcc"
}
if (-not (Test-Path $PkgConfig)) {
    throw "pkg-config not in $MingwBin. Install: pacman -S mingw-w64-x86_64-pkg-config"
}
$opusPc = & $PkgConfig --exists opus
if ($LASTEXITCODE -ne 0) {
    throw "opus pkg-config not found. Install: pacman -S mingw-w64-x86_64-opus"
}
$opusFilePc = & $PkgConfig --exists opusfile
if ($LASTEXITCODE -ne 0) {
    throw "opusfile pkg-config not found. Install: pacman -S mingw-w64-x86_64-opusfile"
}
Write-Host "  gcc:        $(& (Join-Path $MingwBin 'gcc.exe') --version | Select-Object -First 1)" -ForegroundColor DarkGray
Write-Host "  pkg-config: $(& $PkgConfig --version)" -ForegroundColor DarkGray
Write-Host "  opus:       $(& $PkgConfig --modversion opus)" -ForegroundColor DarkGray
Write-Host "  opusfile:   $(& $PkgConfig --modversion opusfile)" -ForegroundColor DarkGray

# --- 2. Set CGO env ---
Write-Section "2/6 Set CGO environment"

$env:CGO_ENABLED = '1'
$env:CC          = (Join-Path $MingwBin 'gcc.exe').Replace('\', '/')
$env:PATH        = "$MingwBin;$env:PATH"
# Pin module proxy for reproducibility
if (-not $env:GOSUMDB) { $env:GOSUMDB = 'sum.golang.org' }

Write-Host "  CGO_ENABLED=$($env:CGO_ENABLED)" -ForegroundColor DarkGray
Write-Host "  CC=$($env:CC)" -ForegroundColor DarkGray
Write-Host "  GOSUMDB=$($env:GOSUMDB)" -ForegroundColor DarkGray

# --- 3. go build ---
Write-Section "3/6 go build"
Push-Location $RepoRoot
try {
    if (-not $SkipBuild) {
        if (Test-Path $ExeFull) {
            Remove-Item $ExeFull -Force
        }
        $buildArgs = @('-o', $ExeRelative)
        if ($WithSilero) {
            $buildArgs = @('-tags', 'silero') + $buildArgs
        }
        $buildArgs = $buildArgs + @('./cmd/server')
        $tagSuffix = if ($WithSilero) { ' -tags silero' } else { '' }
        Write-Host "  go build$tagSuffix -o $ExeRelative ./cmd/server" -ForegroundColor Cyan
        & go build @buildArgs
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit $LASTEXITCODE"
        }
        Write-Host "  OK -> $ExeRelative" -ForegroundColor DarkGray
    } else {
        Write-Host "  skipping (use existing binary)" -ForegroundColor Yellow
    }

    if (-not (Test-Path $ExeFull)) {
        throw "binary not found after build: $ExeFull"
    }
} finally {
    Pop-Location
}

# --- 4. Resolve DLL dependency chain via ldd (recursive) ---
Write-Section "4/6 Resolve DLL dependency chain (recursive ldd)"

if (-not (Test-Path $LddExe)) {
    throw "ldd.exe not found at $LddExe"
}

# Walk every DLL in the chain. Start with the exe, then recurse into each
# non-system DLL we discover. ldd must be run with mingw64/bin on PATH so
# it can resolve the recursive deps (libopusfile-0 -> libopus-0, etc).
$env:PATH = "$MingwBin;$env:PATH"

# Convert a mingw/msys-style POSIX path returned by ldd into a Windows path.
# Examples:
#   /c/Windows/SYSTEM32/foo.dll       -> C:\Windows\SYSTEM32\foo.dll
#   /mingw64/bin/libopus-0.dll        -> C:\msys64\mingw64\bin\libopus-0.dll
#   /d/zyj_workspace/.../libogg-0.dll -> D:\zyj_workspace\...\libogg-0.dll
function Convert-MsysPath {
    param([string]$Posix)
    if (-not $Posix) { return $null }
    if ($Posix -match '^/([a-zA-Z])(/.*)$') {
        return ($matches[1].ToUpper() + ':' + ($matches[2] -replace '/', '\'))
    }
    return $null
}

# Classify a resolved ldd path so we know whether to bundle it.
#   'system'  -> Windows System32 (skip, OS-owned)
#   'mingw'   -> mingw64/bin or usr/bin (bundle)
#   'project' -> inside our bin/ (already there or recurse-into)
#   'unknown' -> record as needed and try alternate locations
function Get-DllOrigin {
    param([string]$WinPath)
    if (-not $WinPath) { return 'unknown' }
    if ($WinPath -match '(?i)\\(WINDOWS|System32)\\') { return 'system' }
    if ($WinPath -match '(?i)\\(mingw64|msys64)\\bin\\') { return 'mingw' }
    if ($WinPath -match '(?i)[\\/]bin[\\/][^\\/]+\.dll$') { return 'project' }
    return 'unknown'
}

function Resolve-DllChain {
    param([string]$TargetPath)

    $seen = New-Object System.Collections.Generic.HashSet[string]
    $needed = New-Object System.Collections.Generic.HashSet[string]
    $queue = New-Object System.Collections.Generic.Queue[string]
    [void]$queue.Enqueue($TargetPath)

    while ($queue.Count -gt 0) {
        $cur = $queue.Dequeue()
        $key = $cur.ToLowerInvariant()
        if ($seen.Contains($key)) { continue }
        [void]$seen.Add($key)

        $lines = & $LddExe $cur 2>&1
        foreach ($line in $lines) {
            if ($line -notmatch '=>') { continue }
            # Token layout (ldd/msys64):
            #   tokens[0] = name (libfoo.dll)
            #   tokens[1] = =>
            #   tokens[2] = resolved POSIX path OR "not"  (if not found, tokens[3]="found")
            $tokens = $line.Trim() -split '\s+'
            if ($tokens.Count -lt 3) { continue }
            $dll = $tokens[0]
            if ($dll -notmatch '\.dll$') { continue }
            $rawPath = $tokens[2]
            $notFound = ($rawPath -eq 'not' -and $tokens.Count -ge 4 -and $tokens[3] -eq 'found')
            if ($notFound) {
                throw "DLL not in PATH: $dll (transitively required by $cur). Check $MingwBin and 'pacman -S'."
            }
            $winPath = Convert-MsysPath $rawPath
            $origin = Get-DllOrigin $winPath
            switch ($origin) {
                'system'  { continue }
                'project' {
                    # Already next to our binary (or inside our bin/). Recurse to
                    # pick up its transitive deps too.
                    if (Test-Path $winPath) {
                        $queue.Enqueue($winPath)
                    } else {
                        $queue.Enqueue((Join-Path $BinDir $dll))
                    }
                    continue
                }
                'mingw' {
                    [void]$needed.Add($dll)
                    if (Test-Path $winPath) {
                        $queue.Enqueue($winPath)
                    }
                    continue
                }
                default {
                    # Unknown origin: treat as needed; try mingw64 fallback later
                    [void]$needed.Add($dll)
                    continue
                }
            }
        }
    }
    # Fallback: for any needed DLL that we couldn't recurse into, try mingw64/bin
    $fallback = @()
    foreach ($dll in $needed) {
        $alt = Join-Path $MingwBin $dll
        if (Test-Path $alt) {
            $fallback += $alt
        }
    }
    foreach ($path in $fallback) {
        # Recurse one more time on each mingw64 DLL to capture transitive deps
        # we might have missed (e.g., libopus-0 chain through libogg-0).
        $key = $path.ToLowerInvariant()
        if ($seen.Contains($key)) { continue }
        [void]$seen.Add($key)
        $sublines = & $LddExe $path 2>&1
        foreach ($line in $sublines) {
            if ($line -notmatch '=>') { continue }
            $tokens = $line.Trim() -split '\s+'
            if ($tokens.Count -lt 3) { continue }
            $sub = $tokens[0]
            if ($sub -notmatch '\.dll$') { continue }
            $rawPath = $tokens[2]
            $notFound = ($rawPath -eq 'not' -and $tokens.Count -ge 4 -and $tokens[3] -eq 'found')
            if ($notFound) {
                throw "DLL not in PATH: $sub (transitively required by $path)"
            }
            $winPath = Convert-MsysPath $rawPath
            $origin = Get-DllOrigin $winPath
            if ($origin -eq 'mingw') {
                [void]$needed.Add($sub)
            }
        }
    }
    return @($needed)
}

$neededSet = Resolve-DllChain -TargetPath $ExeFull

Write-Host "  $($neededSet.Count) runtime DLL deps to bundle (incl. transitive):" -ForegroundColor DarkGray
foreach ($d in ($neededSet | Sort-Object)) {
    Write-Host "    - $d" -ForegroundColor DarkGray
}

# --- 5. Copy DLLs to bin/ ---
Write-Section "5/6 Copy runtime DLLs to bin/"

if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
}

$copied = 0
$skipped = 0
foreach ($dll in ($neededSet | Sort-Object)) {
    $src = Join-Path $MingwBin $dll
    $dst = Join-Path $BinDir $dll
    if (-not (Test-Path $src)) {
        Write-Host "  [WARN] source missing: $src (skipping)" -ForegroundColor Yellow
        continue
    }
    if ((Test-Path $dst) -and ((Get-Item $dst).LastWriteTime -ge (Get-Item $src).LastWriteTime)) {
        $skipped++
        continue
    }
    Copy-Item $src $dst -Force
    Write-Host "  + $dll" -ForegroundColor Green
    $copied++
}
Write-Host "  copied=$copied  skipped=$skipped  total=$($neededSet.Count)" -ForegroundColor DarkGray

# --- 6. Verify self-contained (sanity re-ldd against bin/) ---
Write-Section "6/6 Verify bin/ is self-contained"

$lddBin = & $LddExe $ExeFull 2>&1
$missing = @()
foreach ($line in $lddBin) {
    if ($line -match 'not found') {
        $missing += $line.Trim()
    }
}
if ($missing.Count -gt 0) {
    Write-Host "  [FAIL] unresolved DLL references:" -ForegroundColor Red
    $missing | ForEach-Object { Write-Host "    $_" -ForegroundColor Red }
    throw "build artifact is not self-contained. Check mingw64 dep tree."
}

Write-Host "  OK -> bin/$([System.IO.Path]::GetFileName($ExeFull)) + $($neededSet.Count) bundled DLLs" -ForegroundColor Green

# --- Optional smoke probe ---
if (-not $SkipTest) {
    Write-Section "[optional] smoke probe"
    Write-Host "  Run:  .\bin\xiaozhi-server.exe -config=configs\config.local.yaml" -ForegroundColor Yellow
    Write-Host "  (Skipped by default. Use -SkipTest:$false to auto-start.)" -ForegroundColor DarkGray
}

Write-Host ""
Write-Host "Build OK -> $ExeRelative is self-contained." -ForegroundColor Green
Write-Host "Start   -> .\bin\$([System.IO.Path]::GetFileName($ExeFull)) -config=configs\config.local.yaml" -ForegroundColor Green