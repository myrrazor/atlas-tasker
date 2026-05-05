#!/bin/sh
set -eu

go test -race ./internal/service ./internal/cli ./internal/tui
go test -run=^$ -fuzz=FuzzParse -fuzztime=2s ./internal/slashcmd
go test -run=^$ -fuzz=FuzzParseSearchQuery -fuzztime=2s ./internal/contracts
go test -run=^$ -fuzz=FuzzDecodeTicketMarkdown -fuzztime=2s ./internal/storage/markdown
go test -run=^$ -fuzz=FuzzAutomationStoreLoadRule -fuzztime=2s ./internal/service
go test -run=^$ -fuzz=FuzzReadEventFile -fuzztime=2s ./internal/storage/events
