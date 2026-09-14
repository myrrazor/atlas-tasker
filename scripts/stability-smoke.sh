#!/bin/sh
set -eu

# Count-based fuzzing avoids Go's deadline-cancellation race (golang/go#75804).
# Counts round up the executions measured in PR #155 CI run 34865271389;
# retain the 60s package guard and fail immediately on any real test error.
# See DEC-105 for measurements and the smoke-workload tradeoff.
go test -race ./internal/service ./internal/cli ./internal/tui
go test -timeout 60s -run=^$ -fuzz=FuzzParse -fuzztime=300000x ./internal/slashcmd
go test -timeout 60s -run=^$ -fuzz=FuzzParseSearchQuery -fuzztime=150000x ./internal/contracts
go test -timeout 60s -run=^$ -fuzz=FuzzDecodeTicketMarkdown -fuzztime=80000x ./internal/storage/markdown
go test -timeout 60s -run=^$ -fuzz=FuzzAutomationStoreLoadRule -fuzztime=5000x ./internal/service
go test -timeout 60s -run=^$ -fuzz=FuzzReadEventFile -fuzztime=8000x ./internal/storage/events
