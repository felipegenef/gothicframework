//go:build !js || !wasm

package runtime

func SetText(id, value string)            {}
func SetHTML(id, html string)             {}
func SetValue(id, value string)           {}
func GetValue(id string) string           { return "" }
func GetFileBytes(id string) []byte       { return nil }
func AddClass(id, className string)       {}
func RemoveClass(id, className string)    {}
func ToggleClass(id, className string)    {}
func SetAttr(id, attr, value string)      {}
func SetStyle(id, property, value string) {}

type FetchConfig struct {
	Method    string
	Headers   map[string]string
	Body      string
	BodyBytes []byte
	Query     map[string]string
}

func Fetch(url string, config ...FetchConfig) (string, error) { return "", nil }
func FetchBytes(url string, config ...FetchConfig) ([]byte, error) { return nil, nil }
