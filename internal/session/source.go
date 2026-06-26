package session

// Source identifies which agent component emitted a session-api write. The
// facade is always Omnia-controlled; the runtime may be customer-supplied, so
// the privacy middleware gates runtime-emitted message content separately from
// facade-emitted content. Writers set SourceHeader; the middleware reads it.
const (
	// SourceHeader carries the emitting component on session-api write requests.
	SourceHeader = "X-Omnia-Source"
	// SourceFacade marks writes emitted by the facade.
	SourceFacade = "facade"
	// SourceRuntime marks writes emitted by the runtime.
	SourceRuntime = "runtime"
)
