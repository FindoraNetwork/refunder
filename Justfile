test:
	go test -v --race ./...

e2e:
	go test -v -tags=e2e -count=1 ./e2e/...

lint:
	golangci-lint run

default: lint test
