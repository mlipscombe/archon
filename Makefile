BINARY := bin/archon
ARCHON_IMAGE ?= ghcr.io/mlipscombe/archon:local
SANDBOX_IMAGE ?= ghcr.io/mlipscombe/archon-opencode-sandbox:local

.PHONY: build run config validate fmt tidy clean docker-build docker-build-sandbox docker-push docker-push-sandbox

build:
	go build -o $(BINARY) ./cmd/archon

run: build
	./$(BINARY) start

config: build
	./$(BINARY) config

validate: build
	./$(BINARY) config validate

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

clean:
	rm -rf ./bin

docker-build:
	docker build -t $(ARCHON_IMAGE) -f Dockerfile .

docker-build-sandbox:
	docker build -t $(SANDBOX_IMAGE) -f Dockerfile.sandbox .

docker-push: docker-build
	docker push $(ARCHON_IMAGE)

docker-push-sandbox: docker-build-sandbox
	docker push $(SANDBOX_IMAGE)
