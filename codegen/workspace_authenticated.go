package codegen

// StreamWorkspaceCleanupAuthenticatedInvalidate selects the JavaCard target's
// authenticated persistent stream workspace: only APDUs the applet marks
// authenticated may stage bytes, refusals write nothing persistent and
// streamed responses are refused. It requires stream_workspace = persistent.
// The value equals the target backend's own selector constant.
const StreamWorkspaceCleanupAuthenticatedInvalidate = "authenticated-invalidate"
