.PHONY: build run test vet tidy docker clean

build:
	go build -o bin/telegram-shop ./cmd/bot

run:
	go run ./cmd/bot -config config.local.yaml

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

docker:
	docker compose up -d --build

clean:
	rm -rf bin/
