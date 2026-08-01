.PHONY: run build test vet fmt render docker up down clean

GO ?= go
TYPST ?= typst
ROOT ?= $(CURDIR)

run: ## Run the service locally (needs typst on PATH)
	ROOT=$(ROOT) TYPST_BIN=$(TYPST) $(GO) run ./cmd/server

build: ## Build the server binary into ./bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o bin/cvrenderer ./cmd/server

test: ## Run all tests (render tests skip if typst is absent)
	$(GO) test ./...

test-one: ## Run a single test: make test-one PKG=./internal/typst RUN=TestRenderMissingData
	$(GO) test -run $(RUN) -v $(PKG)

vet: ## go vet
	$(GO) vet ./...

fmt: ## gofmt the tree
	gofmt -w .

render: ## Render data/cv-example.yaml to bin/cv.pdf using typst directly
	$(TYPST) compile --root $(ROOT) --font-path fonts --package-cache-path typst-packages templates/helsinki.typ bin/cv.pdf --input data=/data/cv-example.yaml --input photo=/data/cv-example-photo.svg

docker: ## Build the docker image
	docker build -t cvrenderer:latest .

up: ## Start via docker compose
	docker compose up --build

down: ## Stop docker compose
	docker compose down

clean:
	rm -rf bin
