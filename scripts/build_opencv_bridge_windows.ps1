param()

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$opencvRoot = Join-Path $repoRoot "third_party\opencv\windows-amd64"
$compiler = Get-Command "g++" -ErrorAction Stop
$arguments = @(
    "-shared", "-O2", "-std=c++17", "-DAUTOGO_OPENCV_SHARED",
    "-I", (Join-Path $opencvRoot "include"),
    (Join-Path $repoRoot "opencv_match_bridge.cpp"),
    "-L", (Join-Path $opencvRoot "lib"),
    "-lopencv_imgproc", "-lopencv_core",
    "-o", (Join-Path $opencvRoot "bin\autogo_opencv_bridge.dll")
)
& $compiler.Source @arguments
if ($LASTEXITCODE -ne 0) { throw "OpenCV bridge compilation failed: $LASTEXITCODE" }
Write-Host "Updated embedded OpenCV bridge DLL. No application EXE was built."
