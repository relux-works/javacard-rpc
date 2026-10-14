package codegen

import "github.com/relux-works/javacard-rpc/pluginapi"

// StreamWorkspaceCleanupAuthenticatedInvalidate selects the JavaCard target's
// authenticated persistent stream workspace: only APDUs the applet marks
// authenticated may stage bytes, refusals write nothing persistent and
// streamed responses are refused. It requires stream_workspace = persistent.
// The value equals the target backend's own selector constant.
const StreamWorkspaceCleanupAuthenticatedInvalidate = "authenticated-invalidate"

// validateStreamMode checks the per-method stream_mode. A mode is meaningful
// only inside the authenticated-invalidate runtime, only for a method with a
// request stream, and chunked delivery only for a short result. Unknown or
// misplaced modes are refused, never ignored.
func validateStreamMode(path string, applet Applet, m *Method, add func(path, msg string)) {
	if m.StreamMode == "" {
		return
	}
	path += ".stream_mode"
	switch m.StreamMode {
	case pluginapi.StreamModePublicInvalidate, pluginapi.StreamModeChunked:
	default:
		add(path, "must be public-invalidate or chunked")
		return
	}
	if !m.HasStream() {
		add(path, "applies only to a streamed method")
		return
	}
	if applet.StreamWorkspace != "persistent" || applet.StreamWorkspaceCleanup != StreamWorkspaceCleanupAuthenticatedInvalidate {
		add(path, "requires stream_workspace = persistent and stream_workspace_cleanup = authenticated-invalidate")
		return
	}
	if m.Request.StreamField() == nil {
		add(path, "requires a request stream")
		return
	}
	if m.StreamMode == pluginapi.StreamModeChunked && m.Response != nil && m.Response.StreamField() != nil {
		add(path, "chunked supports short results only; a stream response needs public-invalidate")
	}
}
