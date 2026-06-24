.PHONY: test run-local build-BotLambdaFunction build-CronLambdaFunction

# Local testing
test:
	go test -v ./...

run-local:
	DYNAMODB_ENDPOINT="http://localhost:8000" go run cmd/local-runner/main.go

# AWS SAM custom make builders
build-BotLambdaFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/bot-lambda/main.go

build-CronLambdaFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o $(ARTIFACTS_DIR)/bootstrap ./cmd/cron-lambda/main.go
