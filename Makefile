.PHONY: serve build load test tidy schema docker-up docker-down kill-port

serve: docker-up kill-port
	air

build:
	CGO_ENABLED=0 go build -o fenix.exe ./cmd/fenix

load: docker-up
	CGO_ENABLED=0 go run ./cmd/fenix -cmd load

test:
	CGO_ENABLED=0 go test -v ./cmd/fenix/...

tidy:
	go mod tidy

# Regenerate fenix-config.schema.json from the config structs (run after changing them).
schema:
	go generate ./config/...

docker-up:
	powershell -ExecutionPolicy Bypass -File scripts/start-docker.ps1

docker-down:
	docker-compose -f test/hix-test/docker-compose.yml down

kill-port:
	powershell -ExecutionPolicy Bypass -File scripts/kill-port.ps1
