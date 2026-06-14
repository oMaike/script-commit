@echo off
setlocal

set "PATH=E:\tools\go\bin;C:\Program Files\Git\cmd;%PATH%"
set MIN_HOURS_BETWEEN_COMMITS=23
set HEARTBEAT_FILE=.daily-commit\heartbeat.json
set SOURCE_TEXT_FILE=meu roteiro.txt
set OUTPUT_TEXT_FILE=roteiro.md
set WORD_STATE_FILE=.daily-commit\word-state.json
set TARGET_BRANCH=main
set FORCE_COMMIT=false
set SKIP_PUSH=false

go run .\scripts\daily_commit.go
