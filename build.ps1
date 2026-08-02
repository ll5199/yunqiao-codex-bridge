param(
    [string]$Output = "YunqiaoCodexBridge.exe"
)

$ErrorActionPreference = "Stop"

go test ./...
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags "-s -w -H=windowsgui" -o $Output .

Write-Host "Built $Output"
