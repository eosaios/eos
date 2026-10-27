package webbridge

import "strings"

// coreValueOrNotify invokes the core RPC for a read-only domain snapshot and,
// on failure, pushes one de-duplicated user-visible degradation notice before
// returning the caller-supplied zero. It is the user-facing counterpart of
// coreOnlyValue (which only logs): use it for domains whose empty payload
// would otherwise read as "no data" instead of "load failed" to the user
// (capability lists, usage summary, remote workspaces).
//
// The notice is emitted only when coreReady() is true, so an unconfigured core
// (expected empty) does not produce misleading warnings.
func coreValueOrNotify[T any](s *BridgeService, domain, title, message string, zero T, core func(bridgeRuntimeGateway) (T, error)) T {
	gateway := runtimeGatewayOrNil(s)
	if gateway == nil {
		return zero
	}
	value, err := core(gateway)
	if err == nil {
		s.clearDegraded(domain)
		return value
	}
	if s.coreReady() {
		s.notifyDegraded(domain, title, message)
	}
	return zero
}

// notifyDegraded pushes a single user-visible notice when a read-only core
// domain (usage / capability lists / remote workspaces / ...) could not be
// loaded. It de-duplicates per domain so frequent LoadBootstrap calls do not
// flood the notification feed: the first outage for a domain emits the notice,
// subsequent calls stay quiet until the domain recovers.
//
// This mirrors the modelCatalogFallback pattern: surface the problem to the
// user instead of silently rendering empty data (which reads as "no data"
// rather than "load failed"). Callers must gate on coreReady() so a genuinely
// unconfigured core does not produce misleading notices.
func (s *BridgeService) notifyDegraded(domain, title, message string) {
	if s == nil {
		return
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.degradedNotified == nil {
		s.degradedNotified = map[string]bool{}
	}
	if s.degradedNotified[domain] {
		return
	}
	s.degradedNotified[domain] = true
	s.pushNotificationLocked(title, message, "warning")
}

// clearDegraded resets the de-duplication marker for a domain once its read
// path succeeds again, so a future outage can notify once more.
func (s *BridgeService) clearDegraded(domain string) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.degradedNotified != nil {
		delete(s.degradedNotified, strings.TrimSpace(domain))
	}
}
