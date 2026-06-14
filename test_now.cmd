@echo off
setlocal

set MIN_HOURS_BETWEEN_COMMITS=20
set HEARTBEAT_FILE=.daily-commit\heartbeat.json
set TARGET_BRANCH=main
set FORCE_COMMIT=true
set SKIP_PUSH=true

go run .\scripts\daily_commit.go
