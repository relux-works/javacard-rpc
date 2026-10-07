package backend

import (
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// Both exported constants are usable as string constants in a separate module
// and retain the owner-approved IDL spellings.
func TestCleanupModeNames(t *testing.T) {
	const whole string = pluginapi.StreamWorkspaceCleanupWholeReplyArea
	const written string = pluginapi.StreamWorkspaceCleanupWrittenBytesOnly
	if whole != "whole-reply-area" || written != "written-bytes-only" {
		t.Fatalf("cleanup mode names changed: %q / %q", whole, written)
	}
}

// The public model stores selectors verbatim, including empty/unknown values
// and nonpersistent combinations. It does not normalize or validate metadata;
// downstream persistent cleanup behavior is outside this consumer test.
func TestCleanupMetadataRoundTrip(t *testing.T) {
	for _, tc := range []struct{ storage, cleanup string }{
		{"", ""},
		{"transient", ""},
		{"persistent", ""},
		{"persistent", pluginapi.StreamWorkspaceCleanupWholeReplyArea},
		{"persistent", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly},
		{"persistent", "unknown"},
		{"transient", pluginapi.StreamWorkspaceCleanupWholeReplyArea},
		{"", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly},
	} {
		t.Run(tc.storage+"/"+tc.cleanup, func(t *testing.T) {
			applet := pluginapi.Applet{StreamWorkspace: tc.storage, StreamWorkspaceCleanup: tc.cleanup}
			schema := pluginapi.Schema{Applet: applet}
			var selector string = schema.Applet.StreamWorkspaceCleanup
			if schema.Applet != applet || selector != tc.cleanup || schema.Applet.StreamWorkspace != tc.storage {
				t.Fatalf("metadata altered: %+v", schema.Applet)
			}
		})
	}
}
