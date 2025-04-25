.SILENT:
.PHONY: proto build demo run-scheduler clean

proto:
	mkdir -p gen
	buf generate

build: proto
	mkdir -p bin
	go mod tidy
	cd scheduler && go mod tidy && go build -o ../bin/scheduler ./cmd/server
	go build -o bin/demo ./cmd/demo

demo: build
	./bin/demo

clean:
	rm -rf bin
	rm -rf gen

run-scheduler: build
	./bin/scheduler 