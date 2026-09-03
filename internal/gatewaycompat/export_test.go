// export_test.go exposes the once-per-process announcement guards to the
// external test package, so each row observes its own log output instead of
// inheriting whatever the first row in the binary happened to trigger.
package gatewaycompat

import "sync"

// ResetAnnouncementsForTest rearms both startup announcements.
func ResetAnnouncementsForTest() {
	announceOnce = sync.Once{}
	clampOnce = sync.Once{}
}
