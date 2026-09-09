#Requires -Version 5.1
# Upload dist/cdn/fleetdeck/ to Cloudflare R2 via S3-compatible API.
# Credentials: R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET
# Optional: R2_PUBLIC_BASE (default https://cdn.tarkovbot.com/fleetdeck)
param(
  [string]$SourceDir = "",
  [string]$KeyPrefix = "fleetdeck/",
  [switch]$SkipVerify
)

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
if (-not $ScriptDir) {
  if ($MyInvocation.MyCommand.Path) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
  } else {
    $ScriptDir = (Get-Location).Path
  }
}
$Root = $ScriptDir
while ($Root) {
  if (Test-Path -LiteralPath (Join-Path $Root "deploy\docker-compose.yml")) { break }
  $parent = Split-Path -Parent $Root
  if (-not $parent -or $parent -eq $Root) {
    throw "Could not locate FleetDeck repo root (deploy/docker-compose.yml)."
  }
  $Root = $parent
}

$EnvFile = Join-Path $Root ".env"

function Get-EnvValue([string]$Path, [string]$Key) {
  if (-not (Test-Path -LiteralPath $Path)) { return $null }
  $line = Get-Content -LiteralPath $Path | Where-Object { $_ -match ("^\s*" + [regex]::Escape($Key) + "\s*=") } | Select-Object -First 1
  if (-not $line) { return $null }
  return ($line -replace "^[^=]*=", "").Trim().Trim('"').Trim("'")
}

function Resolve-R2Var([string]$Name) {
  $v = [Environment]::GetEnvironmentVariable($Name)
  if ($v) { return $v.Trim() }
  return Get-EnvValue $EnvFile $Name
}

function Get-CdnContentType([string]$FileName) {
  $base = Split-Path -Leaf $FileName
  $ext = [System.IO.Path]::GetExtension($base).ToLowerInvariant()
  switch ($ext) {
    ".sh" { return "text/x-shellscript" }
    ".json" { return "application/json" }
    ".txt" { return "text/plain" }
    ".md" { return "text/plain" }
    default {
      if ($base -eq "SHA256SUMS" -or $base -like "SHA256SUMS*") { return "text/plain" }
      return "application/octet-stream"
    }
  }
}

function Normalize-KeyPrefix([string]$Prefix) {
  $p = $Prefix.Trim().TrimStart('/')
  if (-not $p) { return "fleetdeck/" }
  if (-not $p.EndsWith("/")) { $p += "/" }
  return $p
}

$AccountId = Resolve-R2Var "R2_ACCOUNT_ID"
$AccessKey = Resolve-R2Var "R2_ACCESS_KEY_ID"
$SecretKey = Resolve-R2Var "R2_SECRET_ACCESS_KEY"
$Bucket = Resolve-R2Var "R2_BUCKET"
$PublicBase = Resolve-R2Var "R2_PUBLIC_BASE"
if (-not $PublicBase) { $PublicBase = "https://cdn.tarkovbot.com/fleetdeck" }
$PublicBase = $PublicBase.TrimEnd('/')

$missing = @()
if (-not $AccountId) { $missing += "R2_ACCOUNT_ID" }
if (-not $AccessKey) { $missing += "R2_ACCESS_KEY_ID" }
if (-not $SecretKey) { $missing += "R2_SECRET_ACCESS_KEY" }
if (-not $Bucket) { $missing += "R2_BUCKET" }
if ($missing.Count -gt 0) {
  throw @"
Missing R2 credentials: $($missing -join ', ').
Set them in the environment or in .env (see .env.example).
Create an R2 API token in Cloudflare Dashboard -> R2 -> Manage R2 API Tokens
(Object Read & Write on your public CDN bucket).
"@
}

if (-not $SourceDir) { $SourceDir = Join-Path $Root "dist\cdn\fleetdeck" }
if (-not (Test-Path -LiteralPath $SourceDir)) {
  throw "Source dir not found: $SourceDir (run .\scripts\publish-cdn.ps1 first, or use -Upload)."
}

$KeyPrefix = Normalize-KeyPrefix $KeyPrefix
$Endpoint = "https://$AccountId.r2.cloudflarestorage.com"
$files = @(Get-ChildItem -LiteralPath $SourceDir -Recurse -File)
if ($files.Count -eq 0) {
  throw "No files under $SourceDir"
}

Write-Host "Uploading $($files.Count) file(s) from $SourceDir"
Write-Host "  endpoint: $Endpoint"
Write-Host "  bucket:   $Bucket"
Write-Host "  prefix:   $KeyPrefix"
Write-Host "  public:   $PublicBase"

function Get-RelativeKey([System.IO.FileInfo]$File, [string]$RootDir, [string]$Prefix) {
  $fullRoot = (Resolve-Path -LiteralPath $RootDir).Path.TrimEnd('\', '/')
  $rel = $File.FullName.Substring($fullRoot.Length).TrimStart('\', '/').Replace('\', '/')
  return "$Prefix$rel"
}

function Test-AwsCli {
  $cmd = Get-Command aws -ErrorAction SilentlyContinue
  return [bool]$cmd
}

function Invoke-AwsS3Upload {
  $prevAccess = $env:AWS_ACCESS_KEY_ID
  $prevSecret = $env:AWS_SECRET_ACCESS_KEY
  $prevRegion = $env:AWS_DEFAULT_REGION
  $prevPager = $env:AWS_PAGER
  try {
    $env:AWS_ACCESS_KEY_ID = $AccessKey
    $env:AWS_SECRET_ACCESS_KEY = $SecretKey
    $env:AWS_DEFAULT_REGION = "auto"
    $env:AWS_PAGER = ""

    $i = 0
    foreach ($f in $files) {
      $i++
      $key = Get-RelativeKey $f $SourceDir $KeyPrefix
      $ctype = Get-CdnContentType $f.Name
      $dest = "s3://$Bucket/$key"
      Write-Host ("  [{0}/{1}] {2} ({3})" -f $i, $files.Count, $key, $ctype)
      & aws s3 cp $f.FullName $dest `
        --endpoint-url $Endpoint `
        --content-type $ctype `
        --only-show-errors
      if ($LASTEXITCODE -ne 0) {
        throw "aws s3 cp failed for $key (exit $LASTEXITCODE)"
      }
    }
  } finally {
    if ($null -eq $prevAccess) { Remove-Item Env:AWS_ACCESS_KEY_ID -ErrorAction SilentlyContinue } else { $env:AWS_ACCESS_KEY_ID = $prevAccess }
    if ($null -eq $prevSecret) { Remove-Item Env:AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue } else { $env:AWS_SECRET_ACCESS_KEY = $prevSecret }
    if ($null -eq $prevRegion) { Remove-Item Env:AWS_DEFAULT_REGION -ErrorAction SilentlyContinue } else { $env:AWS_DEFAULT_REGION = $prevRegion }
    if ($null -eq $prevPager) { Remove-Item Env:AWS_PAGER -ErrorAction SilentlyContinue } else { $env:AWS_PAGER = $prevPager }
  }
}

function Get-SigV4Signature([byte[]]$Key, [string]$Data) {
  $hmac = New-Object System.Security.Cryptography.HMACSHA256
  $hmac.Key = $Key
  return $hmac.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($Data))
}

function Invoke-R2PutObject([string]$Key, [string]$FilePath, [string]$ContentType) {
  $region = "auto"
  $service = "s3"
  $amzDate = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
  $dateStamp = $amzDate.Substring(0, 8)
  $payloadBytes = [System.IO.File]::ReadAllBytes($FilePath)
  $sha256 = [System.Security.Cryptography.SHA256]::Create()
  $payloadHash = ($sha256.ComputeHash($payloadBytes) | ForEach-Object { $_.ToString("x2") }) -join ""

  $hostHeader = "$AccountId.r2.cloudflarestorage.com"
  $encodedKey = ($Key -split "/" | ForEach-Object { [Uri]::EscapeDataString($_) }) -join "/"
  $canonicalUri = "/$Bucket/$encodedKey"
  $canonicalQuerystring = ""
  $canonicalHeaders = "content-type:$ContentType`nhost:$hostHeader`nx-amz-content-sha256:$payloadHash`nx-amz-date:$amzDate`n"
  $signedHeaders = "content-type;host;x-amz-content-sha256;x-amz-date"
  $canonicalRequest = "PUT`n$canonicalUri`n$canonicalQuerystring`n$canonicalHeaders`n$signedHeaders`n$payloadHash"

  $algorithm = "AWS4-HMAC-SHA256"
  $credentialScope = "$dateStamp/$region/$service/aws4_request"
  $canonicalRequestHash = ($sha256.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($canonicalRequest)) | ForEach-Object { $_.ToString("x2") }) -join ""
  $stringToSign = "$algorithm`n$amzDate`n$credentialScope`n$canonicalRequestHash"

  $kDate = Get-SigV4Signature ([System.Text.Encoding]::UTF8.GetBytes("AWS4$SecretKey")) $dateStamp
  $kRegion = Get-SigV4Signature $kDate $region
  $kService = Get-SigV4Signature $kRegion $service
  $kSigning = Get-SigV4Signature $kService "aws4_request"
  $signature = ((Get-SigV4Signature $kSigning $stringToSign) | ForEach-Object { $_.ToString("x2") }) -join ""

  $authorization = "$algorithm Credential=$AccessKey/$credentialScope, SignedHeaders=$signedHeaders, Signature=$signature"
  $uri = "$Endpoint$canonicalUri"

  $req = [System.Net.HttpWebRequest]::Create($uri)
  $req.Method = "PUT"
  $req.ContentType = $ContentType
  $req.ContentLength = $payloadBytes.Length
  $req.Headers.Add("x-amz-content-sha256", $payloadHash)
  $req.Headers.Add("x-amz-date", $amzDate)
  $req.Headers.Add("Authorization", $authorization)
  $stream = $req.GetRequestStream()
  $stream.Write($payloadBytes, 0, $payloadBytes.Length)
  $stream.Close()
  try {
    $resp = $req.GetResponse()
    $code = [int]$resp.StatusCode
    $resp.Close()
    if ($code -lt 200 -or $code -ge 300) {
      throw "R2 PUT $Key returned HTTP $code"
    }
  } catch [System.Net.WebException] {
    $msg = $_.Exception.Message
    if ($_.Exception.Response) {
      $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
      $body = $reader.ReadToEnd()
      $reader.Close()
      throw "R2 PUT failed for $Key : $msg`n$body"
    }
    throw
  }
}

function Invoke-PowerShellUpload {
  Write-Host "aws CLI not found - using PowerShell SigV4 upload."
  Write-Host "  Tip: install AWS CLI for faster uploads: https://aws.amazon.com/cli/"
  $i = 0
  foreach ($f in $files) {
    $i++
    $key = Get-RelativeKey $f $SourceDir $KeyPrefix
    $ctype = Get-CdnContentType $f.Name
    Write-Host ("  [{0}/{1}] {2} ({3})" -f $i, $files.Count, $key, $ctype)
    Invoke-R2PutObject -Key $key -FilePath $f.FullName -ContentType $ctype
  }
}

if (Test-AwsCli) {
  Write-Host "Using aws CLI (S3-compatible -> R2)..."
  Invoke-AwsS3Upload
} else {
  Invoke-PowerShellUpload
}

Write-Host "Upload complete."

if (-not $SkipVerify) {
  $checks = @(
    "$PublicBase/config.json",
    "$PublicBase/install.sh",
    "$PublicBase/install-user.sh",
    "$PublicBase/upgrade.sh"
  )
  Write-Host "Verifying public CDN URLs..."
  foreach ($url in $checks) {
    try {
      $r = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 20 -Method Head
      Write-Host ("  OK {0} {1}" -f $r.StatusCode, $url)
    } catch {
      try {
        $r = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 20
        Write-Host ("  OK {0} {1}" -f $r.StatusCode, $url)
      } catch {
        Write-Warning ("  VERIFY FAILED {0}: {1}" -f $url, $_.Exception.Message)
      }
    }
  }
}
