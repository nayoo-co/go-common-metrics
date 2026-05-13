package metrics

import "runtime"

// runtimeVersion is split out so tests can override (and so the buildinfo
// file stays focused on registration vs runtime introspection).
func runtimeVersion() string {
	return runtime.Version()
}
