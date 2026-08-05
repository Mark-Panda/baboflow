PROTOC = protoc
API_PROTO_FILES := $(shell find api -name '*.proto' -type f)
API_GENERATED_FILES := $(patsubst %.proto,%.pb.go,$(API_PROTO_FILES)) \
	$(patsubst %.proto,%_grpc.pb.go,$(API_PROTO_FILES)) \
	$(patsubst %.proto,%_http.pb.go,$(API_PROTO_FILES))

.PHONY: init api api-smoke generate build guard test

init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@v2.0.0-20251205160234-b9fab9a5a5ab
	go install github.com/google/wire/cmd/wire@v0.7.0

api:
	command -v $(PROTOC) >/dev/null
	command -v protoc-gen-go >/dev/null
	command -v protoc-gen-go-grpc >/dev/null
	command -v protoc-gen-go-http >/dev/null
	rm -f $(API_GENERATED_FILES)
	$(PROTOC) --proto_path=api --proto_path=third_party \
		--go_out=paths=source_relative:api \
		--go-grpc_out=paths=source_relative:api \
		--go-http_out=paths=source_relative:api \
		$(API_PROTO_FILES)

api-smoke: api
	@tmp_dir=$$(mktemp -d); \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	test -f api/baboflow/v1/common.pb.go; \
	test -f api/baboflow/v1/auth.pb.go; \
	test -f api/baboflow/v1/auth_grpc.pb.go; \
	test -f api/baboflow/v1/auth_http.pb.go; \
	before_files=$$(find api -type f -name '*.pb.go' -print | sort); \
	before=$$(printf '%s\n' "$$before_files" | xargs shasum -a 256 | shasum -a 256); \
	mkdir -p "$$tmp_dir/out-1" "$$tmp_dir/out-2"; \
	printf '%s\n' \
		'syntax = "proto3";' \
		'package smoke.v1;' \
		'import "google/api/annotations.proto";' \
		'option go_package = "baboflow/.superpowers/sdd/protosmoke;protosmoke";' \
		'message Request {}' \
		'message Response {}' \
		'service SmokeService {' \
		'  rpc Check(Request) returns (Response) {' \
		'    option (google.api.http) = { get: "/smoke" };' \
		'  }' \
		'}' > "$$tmp_dir/smoke.proto"; \
	for out in "$$tmp_dir/out-1" "$$tmp_dir/out-2"; do \
		$(PROTOC) --proto_path="$$tmp_dir" --proto_path=third_party \
			--go_out=paths=source_relative:"$$out" \
			--go-grpc_out=paths=source_relative:"$$out" \
			--go-http_out=paths=source_relative:"$$out" \
			"$$tmp_dir/smoke.proto"; \
		test -f "$$out/smoke.pb.go"; \
		test -f "$$out/smoke_grpc.pb.go"; \
		test -f "$$out/smoke_http.pb.go"; \
	done; \
	diff -ru "$$tmp_dir/out-1" "$$tmp_dir/out-2"; \
	$(MAKE) api >/dev/null; \
	after_files=$$(find api -type f -name '*.pb.go' -print | sort); \
	test "$$before_files" = "$$after_files"; \
	after=$$(printf '%s\n' "$$after_files" | xargs shasum -a 256 | shasum -a 256); \
	test "$$before" = "$$after"

generate: api
	cd cmd/baboflow && wire

build:
	go build ./cmd/baboflow

guard:
	@python3 scripts/guard_legacy_http.py

test:
	go test ./...
