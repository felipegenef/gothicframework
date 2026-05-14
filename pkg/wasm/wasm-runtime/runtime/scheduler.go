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


