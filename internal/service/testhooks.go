package service

import "github.com/myrrazor/atlas-tasker/internal/contracts"

var testBeforeRunWorktreeCreateHook func(contracts.RunSnapshot) error
var testRuntimeArtifactWriteHook func(string) error
var testEvidenceArtifactCopyHook func(string, int64) error
