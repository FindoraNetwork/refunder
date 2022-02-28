test:
	go test -v --race ./...

e2e:
	# need to run the e2e tests one by one or will facing 
	# `send_raw_transaction: broadcast_tx_sync failed` error
	go test -v -tags=e2e -count=1 ./e2e/giveaway/...
	go test -v -tags=e2e -count=1 ./e2e/gasfee/...

lint:
	golangci-lint run

default: lint test
