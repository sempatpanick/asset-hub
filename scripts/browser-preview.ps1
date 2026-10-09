param([int]$Port=55433)
$ErrorActionPreference='Stop'
$taskRoot=Split-Path -Parent $PSScriptRoot
$taskEnvPath=Join-Path $taskRoot '.local/test.env'
if(-not(Test-Path -LiteralPath $taskEnvPath)){throw 'Start dedicated test containers before running the preview.'}
$taskPassword=(Get-Content -LiteralPath $taskEnvPath | Where-Object {$_ -like 'TEST_POSTGRES_PASSWORD=*'}).Substring(23)
$env:TEST_DATABASE_URL='postgres://assethub:'+$taskPassword+'@127.0.0.1:'+$Port+'/postgres?sslmode=disable'
$env:GOTOOLCHAIN='go1.27.2'
Push-Location (Join-Path $taskRoot 'services/backend')
try { go run ./integration/preview; if($LASTEXITCODE -ne 0){throw 'Browser preview failed'} } finally { Pop-Location; Remove-Item Env:TEST_DATABASE_URL }
