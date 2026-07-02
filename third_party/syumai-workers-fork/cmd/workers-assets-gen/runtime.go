package main

type Runtime string

const (
	RuntimeCloudflare Runtime = "cloudflare"
	RuntimeBrowser    Runtime = "browser"
)

func (r Runtime) IsValid() bool {
	switch r {
	case RuntimeCloudflare, RuntimeBrowser:
		return true
	}
	return false
}

func (r Runtime) AssetFileName() string {
	return string(r) + ".mjs"
}
