//go:build js && wasm

package runtime

var batchDepth int
var pendingSubscriptions []*Subscription

func addPendingSubscription(e *Subscription) {
	for _, pe := range pendingSubscriptions {
		if pe == e {
			return
		}
	}
	pendingSubscriptions = append(pendingSubscriptions, e)
}

// BeginBatch defers observer notifications until the matching EndBatch.
// Nested calls are supported; effects only flush when depth returns to 0.
// Each pending Subscription is deduplicated and runs at most once per batch.
func BeginBatch() {
	batchDepth++
}

// EndBatch decrements the batch depth and, when it reaches zero,
// runs every Subscription that was queued during the batch (deduplicated).
func EndBatch() {
	if batchDepth == 0 {
		return
	}
	batchDepth--
	if batchDepth > 0 {
		return
	}
	pending := pendingSubscriptions
	pendingSubscriptions = nil
	for _, e := range pending {
		e.run()
	}
}
