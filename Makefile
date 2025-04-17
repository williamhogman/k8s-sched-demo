.PHONY: proto build run-global-scheduler run-scheduler clean

proto:
	mkdir -p gen
	, buf generate

build: proto
	mkdir -p bin
	go mod tidy
	cd global-scheduler && go mod tidy && go build -o ../bin/global-scheduler ./cmd/server
	cd scheduler && go mod tidy && go build -o ../bin/scheduler ./cmd/server
	go build -o bin/client ./cmd/client

clean:
	rm -rf bin
	rm -rf gen

run-global-scheduler: build
	./bin/global-scheduler

run-scheduler: build
	./bin/scheduler

run-client: build
	./bin/client 