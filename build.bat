@echo off
REM CertWatch Agent Build Script for Windows

setlocal enabledelayedexpansion

REM Set default values
set VERSION=dev
set GIT_COMMIT=unknown
set BUILD_DATE=%date:~10,4%-%date:~4,2%-%date:~7,2%

REM Try to get version from git
for /f "tokens=*" %%i in ('git describe --tags --always --dirty 2^>nul') do set VERSION=%%i
for /f "tokens=*" %%i in ('git rev-parse --short HEAD 2^>nul') do set GIT_COMMIT=%%i

set LDFLAGS=-X github.com/certwatch-app/cw-agent/internal/version.Version=%VERSION% -X github.com/certwatch-app/cw-agent/internal/version.GitCommit=%GIT_COMMIT% -X github.com/certwatch-app/cw-agent/internal/version.BuildDate=%BUILD_DATE%

if "%1"=="" goto build
if "%1"=="build" goto build
if "%1"=="build-windows" goto build-windows
if "%1"=="clean" goto clean
if "%1"=="test" goto test
if "%1"=="deps" goto deps
if "%1"=="tidy" goto tidy
if "%1"=="version" goto version
if "%1"=="help" goto help
goto help

:build
echo Building cw-agent...
if not exist bin mkdir bin
go build -ldflags "%LDFLAGS%" -o bin/cw-agent.exe ./cmd/cw-agent
if %errorlevel% neq 0 exit /b %errorlevel%
echo Build complete: bin\cw-agent.exe
goto end

:build-windows
echo Building cw-agent for Windows amd64...
if not exist bin mkdir bin
set GOOS=windows
set GOARCH=amd64
go build -ldflags "%LDFLAGS%" -o bin/cw-agent-windows-amd64.exe ./cmd/cw-agent
if %errorlevel% neq 0 exit /b %errorlevel%
echo Build complete: bin\cw-agent-windows-amd64.exe
goto end

:clean
echo Cleaning...
if exist bin rmdir /s /q bin
if exist coverage.out del coverage.out
echo Clean complete
goto end

:test
echo Running tests...
go test -v -race ./...
goto end

:deps
echo Downloading dependencies...
go mod download
goto end

:tidy
echo Tidying go.mod...
go mod tidy
goto end

:version
echo Version: %VERSION%
echo Commit: %GIT_COMMIT%
goto end

:help
echo CertWatch Agent Build Script
echo.
echo Usage: build.bat [target]
echo.
echo Targets:
echo   build          Build the binary (default)
echo   build-windows  Build for Windows amd64
echo   clean          Clean build artifacts
echo   test           Run tests
echo   deps           Download dependencies
echo   tidy           Tidy go.mod
echo   version        Show version info
echo   help           Show this help
goto end

:end
endlocal
