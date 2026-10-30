param([switch]$Child, [string]$Mode, [string]$Fixture)
$ErrorActionPreference = 'Stop'
if ($Child) {
    function Invoke-WebRequest {
        param([switch]$UseBasicParsing, $Uri, $OutFile)
        $base = 'https://github.com/specscore/specscore-cli/releases/download/v1.2.3'
        switch ($Uri) {
            "$base/specscore_1.2.3_windows_amd64.zip" { Copy-Item "$Fixture/archive.zip" $OutFile }
            "$base/specscore_1.2.3_checksums.txt" {
                if ($Mode -eq 'missing') { throw 'manifest unavailable' }
                Copy-Item "$Fixture/checksums.txt" $OutFile
            }
            default { throw "unexpected release URL: $Uri" }
        }
    }
    $env:PROCESSOR_ARCHITECTURE = 'AMD64'
    $env:SPECSCORE_VERSION = 'v1.2.3'
    $env:SPECSCORE_SKIP_USER_PATH = '1'
    $env:SPECSCORE_INSTALL_DIR = Join-Path $Fixture 'bin'
    . (Join-Path $PSScriptRoot 'install.ps1')
    exit 0
}
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("specscore-test-" + [guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null
try {
    Set-Content (Join-Path $tmp 'specscore.exe') 'fixture'
    Compress-Archive -Path (Join-Path $tmp 'specscore.exe') -DestinationPath (Join-Path $tmp 'archive.zip')
    $hash = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp 'archive.zip')).Hash.ToLowerInvariant()
    Set-Content (Join-Path $tmp 'checksums.txt') "$hash  specscore_1.2.3_windows_amd64.zip"
    & pwsh -NoProfile -File $PSCommandPath -Child -Mode success -Fixture $tmp
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path (Join-Path $tmp 'bin/specscore.exe'))) { throw 'valid install failed' }
    Remove-Item (Join-Path $tmp 'bin/specscore.exe')
    & pwsh -NoProfile -File $PSCommandPath -Child -Mode missing -Fixture $tmp
    if ($LASTEXITCODE -eq 0) { throw 'missing manifest accepted' }
    Set-Content (Join-Path $tmp 'checksums.txt') (('0' * 64) + '  specscore_1.2.3_windows_amd64.zip')
    & pwsh -NoProfile -File $PSCommandPath -Child -Mode mismatch -Fixture $tmp
    if ($LASTEXITCODE -eq 0 -or (Test-Path (Join-Path $tmp 'bin/specscore.exe'))) { throw 'checksum mismatch accepted' }
    Set-Content (Join-Path $tmp 'checksums.txt') (('0' * 64) + '  other.zip')
    & pwsh -NoProfile -File $PSCommandPath -Child -Mode entry -Fixture $tmp
    if ($LASTEXITCODE -eq 0) { throw 'missing entry accepted' }
} finally {
    Remove-Item -Recurse -Force $tmp
}

exit 0
