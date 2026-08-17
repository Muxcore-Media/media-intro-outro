.PHONY: test build proto update-goldens
test:
	CGO_ENABLED=0 go test ./...
update-goldens:
	CGO_ENABLED=0 go test ./internal/ -run TestDetectGoldenFixtures -update-goldens
build:
	CGO_ENABLED=0 go build -o bin/media-intro-outro ./cmd/module
proto:
	PATH="$$HOME/go/bin:$$HOME/.local/protoc/bin:$$PATH" protoc -I proto \
		--go_out=proto/gen --go_opt=paths=source_relative \
		--go-grpc_out=proto/gen --go-grpc_opt=paths=source_relative \
		proto/muxcore/introoutro/v1/introoutro.proto
