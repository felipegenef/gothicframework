//go:build js && wasm

package runtime

var batchDepth int
var pendingEffects []*Effect

func addPending(e *Effect) {
	for _, pe := range pendingEffects {
		if pe == e {
			return
		}
	}
	pendingEffects = append(pendingEffects, e)
}

func Batch(fn func()) {
	batchDepth++
	fn()
	batchDepth--
	if batchDepth != 0 {
		return
	}
	pending := make([]*Effect, len(pendingEffects))
	copy(pending, pendingEffects)
	pendingEffects = pendingEffects[:0]
	for _, e := range pending {
		e.run()
	}
}

