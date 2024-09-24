.PHONY: tidy
tidy: 
	go mod tidy
	go fmt ./...

.PHONY: test
test:
	go test -v ./...

.PHONY: clean
clean: 
	rm -rf dist/ csi-utho-plugin

.PHONY: deploy
deploy: build docker-build docker-push

.PHONY: build
build:
	@echo "building utho csi for linux"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-X main.version=$(VERSION)' -o csi-utho-plugin ./cmd/csi-utho

.PHONY: docker-build
docker-build:
	@echo "building docker image to dockerhub $(REGISTRY) with version $(VERSION)"
	docker build . -t hmada15/csi-utho:$(VERSION)
	docker tag hmada15/csi-utho:$(VERSION) hmada15/csi-utho:latest

.PHONY: docker-push
docker-push:
	docker push hmada15/csi-utho:$(VERSION)
	docker push hmada15/csi-utho:latest
