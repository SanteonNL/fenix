.PHONY: serve build load test tidy docker-up docker-down kill-port deident-demo deident-pipeline-demo

serve: docker-up kill-port
	air

build:
	CGO_ENABLED=0 go build -o fenix.exe ./cmd/fenix

# Usage: make load [SOURCE=zbj-test] or [SOURCE="zbj-test,pja,sim"]
load: docker-up
	CGO_ENABLED=0 go run ./cmd/fenix -cmd load $(SOURCE)

test:
	CGO_ENABLED=0 go test -v ./cmd/fenix/... ./internal/deident/...

# Prints a Patient + Observation before/after internal/deident, with the
# coverage report in between — see cmd/deidentdemo.
deident-demo:
	CGO_ENABLED=0 go run ./cmd/deidentdemo

# Runs the real fenix CLI pipeline (load -> convert -> de-identify) end to
# end against test/data/sim, using config/deident-demo.yaml and the ruleset
# at config/deident/sim-demo-ruleset.json. Output lands in
# output/deident-demo/{patient,observation}.json. The key below is a fixed
# demo-only value (base64 of "demo-only-not-a-real-secret") — never a real
# secret; a real deployment always sources FENIX_DEIDENT_KEY from .env.
deident-pipeline-demo:
	FENIX_DEIDENT_KEY=ZGVtby1vbmx5LW5vdC1hLXJlYWwtc2VjcmV0 CGO_ENABLED=0 go run ./cmd/fenix -config config/deident-demo.yaml -cmd all

tidy:
	go mod tidy

docker-up:
	powershell -ExecutionPolicy Bypass -File scripts/start-docker.ps1

docker-down:
	docker-compose -f test/hix-test/docker-compose.yml down

kill-port:
	powershell -ExecutionPolicy Bypass -File scripts/kill-port.ps1
