package gothicwasm

import . "github.com/felipegenef/gothicframework/v2/pkg/wasm"

type CounterState struct {
	Count int
}

var _ = CreateTopic(CounterState{}, TopicConfig{
	Name:             "counter",
	Compression:      BROTLI,
	SubscriberFnName: "GetCounterTopic",
	ComponentFnName:  "MountCounterTopic",
})
