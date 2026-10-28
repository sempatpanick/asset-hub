param([int]$Port=55433)
$ErrorActionPreference='Stop'
$taskRoot=Split-Path -Parent $PSScriptRoot
$taskEnvPath=Join-Path $taskRoot '.local/test.env'
if(-not(Test-Path -LiteralPath $taskEnvPath)){throw 'Create .local/test.env with a unique TEST_POSTGRES_PASSWORD, then start deploy/compose.test.yaml.'}
$taskPassword=(Get-Content -LiteralPath $taskEnvPath | Where-Object {$_ -like 'TEST_POSTGRES_PASSWORD=*'}).Substring(23)
$env:TEST_DATABASE_URL='postgres://assethub:'+$taskPassword+'@127.0.0.1:'+$Port+'/postgres?sslmode=disable'
$env:GOTOOLCHAIN='go1.27.3'
$env:TEST_REDIS_URL='redis://127.0.0.1:56380/11'
$taskPgBin=Join-Path $taskRoot '.local/pg-runtime/pgsql/bin'
if(Test-Path -LiteralPath (Join-Path $taskPgBin 'pg_dump.exe')){
  $env:TEST_PG_DUMP_BINARY=Join-Path $taskPgBin 'pg_dump.exe'
  $env:TEST_PG_RESTORE_BINARY=Join-Path $taskPgBin 'pg_restore.exe'
}
Push-Location (Join-Path $taskRoot 'services/backend')
try { go test ./integration ./cmd/admin -count=1 -v; if($LASTEXITCODE -ne 0){throw 'Integration tests failed'} } finally { Pop-Location; Remove-Item Env:TEST_DATABASE_URL; Remove-Item Env:TEST_REDIS_URL; Remove-Item Env:TEST_PG_DUMP_BINARY,Env:TEST_PG_RESTORE_BINARY -ErrorAction SilentlyContinue }
