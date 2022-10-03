package main

import (
	"reflect"
	"testing"
)

func TestWhenForceTeamIDSpecifiedTheCreateArchiveCmdAddsDEVELOPMENT_TEAM(t *testing.T) {
	opts := ArchiveCommandOpts{
		ForceTeamID: "ABCD",
	}

	cmd := createArchiveCmd(opts)
	got := cmd.PrintableCmd()
	want := `xcodebuild "archive" "-destination" "generic/platform=macOS" "DEVELOPMENT_TEAM=ABCD"`
	if !reflect.DeepEqual(got, want) {
		t.Errorf("createArchiveCmd() = %v, want %v", got, want)
	}
}
