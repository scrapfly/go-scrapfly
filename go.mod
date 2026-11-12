module github.com/scrapfly/go-scrapfly

go 1.25.0

// Documents proxy pool names the API does not accept.
retract v1.7.0

require (
	github.com/PuerkitoBio/goquery v1.13.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
)

require github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect

require (
	github.com/andybalholm/cascadia v1.3.4 // indirect
	github.com/google/jsonschema-go v0.4.3
	golang.org/x/net v0.58.0 // indirect
)
