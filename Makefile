.PHONY: tidy
tidy: 
	go mod tidy
	go fmt ./...

.PHONY: clean
clean: 
	rm -rf dist/ csi-utho-plugin

# make new-deploy VERSION=0.1.16
.PHONY: new-deploy
new-deploy: push
	@sed -i 's|\(utho/csi-utho:\)[0-9]*\.[0-9]*\.[0-9]*|\1$(VERSION)|g' deploy/utho.yml
	@kubectl apply -f deploy/secret.yaml
	@kubectl apply -f deploy/utho.yml

.PHONY: push
push: build docker-build docker-push

.PHONY: build
build:
	@echo "building utho csi for linux"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-X main.version=$(VERSION)' -o csi-utho-plugin ./cmd/csi-utho

.PHONY: docker-build
docker-build:
	@echo "building docker image to dockerhub utho/csi-utho with version $(VERSION)"
	docker build . -t utho/csi-utho:$(VERSION)
	docker tag utho/csi-utho:$(VERSION) utho/csi-utho:latest

.PHONY: docker-push
docker-push:
	docker push utho/csi-utho:$(VERSION)
	docker push utho/csi-utho:latest

.PHONY: deploy
deploy:
	@kubectl apply -f deploy/utho.yml

.PHONY: undeploy
undeploy:
	kubectl delete -f deploy/utho.yml

.PHONY: test
test:
	csi-sanity --ginkgo.focus="$(FILTER)" --csi.endpoint=unix:///var/lib/csi/sockets/pluginproxy/csi.sock -csi.testvolumeparameters=create.yaml  --ginkgo.junit-report=test.xml --ginkgo.v

.PHONY: test-all
test-all:
	csi-sanity --csi.endpoint=unix:///var/lib/csi/sockets/pluginproxy/csi.sock -csi.testvolumeparameters=create.yaml  --ginkgo.junit-report=test.xml --ginkgo.v

.PHONY: html
html:
	junit2html test.xml
	firefox test.xml.html
