#!/bin/sh
set -eu

# Each fuzz target needs a test timeout well above -fuzztime. Go cancels the
# fuzz context when the time elapses; if the package timeout is only about
# fuzztime+1s, loaded CI runners fail the target with "context deadline
# exceeded" even though no input crashed. That flake already hit FuzzParse
# and FuzzReadEventFile on this stack.
go test -race ./internal/service ./internal/cli ./internal/tui
go test -timeout 60s -run=^$ -fuzz=FuzzParse -fuzztime=2s ./internal/slashcmd
go test -timeout 60s -run=^$ -fuzz=FuzzParseSearchQuery -fuzztime=2s ./internal/contracts
go test -timeout 60s -run=^$ -fuzz=FuzzDecodeTicketMarkdown -fuzztime=2s ./internal/storage/markdown
go test -timeout 60s -run=^$ -fuzz=FuzzAutomationStoreLoadRule -fuzztime=2s ./internal/service
go test -timeout 60s -run=^$ -fuzz=FuzzReadEventFile -fuzztime=2s ./internal/storage/events
