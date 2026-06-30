.PHONY: serve build prepare test tidy docker-up docker-down kill-port

serve: docker-up kill-port
	air

build:
	CGO_ENABLED=0 go build -o fenix.exe ./cmd/fenix

prepare: docker-up
	CGO_ENABLED=0 go run ./cmd/fenix -cmd prepare

test:
	CGO_ENABLED=0 go test -v ./cmd/fenix/...

tidy:
	go mod tidy

docker-up:
	powershell -ExecutionPolicy Bypass -File scripts/start-docker.ps1

docker-down:
	docker-compose -f test/hix-test/docker-compose.yml down

kill-port:
	powershell -ExecutionPolicy Bypass -File scripts/kill-port.ps1
