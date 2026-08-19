module github.com/mevansam/context-mesh-engine

go 1.25.7

require (
	github.com/google/jsonschema-go v0.4.3
	github.com/modelcontextprotocol/go-sdk v0.0.0
	github.com/pb33f/libopenapi v0.38.7
	go.yaml.in/yaml/v4 v4.0.0-rc.6
	golang.org/x/mod v0.40.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/pb33f/jsonpath v0.8.3 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/modelcontextprotocol/go-sdk => ../go-sdk

replace github.com/pb33f/libopenapi => ../libopenapi
