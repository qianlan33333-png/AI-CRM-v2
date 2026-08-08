// Package platformhttp owns process-level HTTP handlers and adapters.
//
// Domain HTTP packages remain private to their domain. This platform package
// must not acquire domain behavior, readiness probes, or listener lifecycle.
package platformhttp
