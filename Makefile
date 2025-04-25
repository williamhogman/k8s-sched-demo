.SILENT:
.PHONY: proto build demo run-scheduler run-activator clean docker-build k8s-deploy

proto:
	mkdir -p gen
	buf generate

build: proto
	mkdir -p bin
	go mod tidy
	cd scheduler && go mod tidy && go build -o ../bin/scheduler ./cmd/server
	cd activator && go mod tidy && go build -o ../bin/activator ./cmd/activator
	go build -o bin/demo ./cmd/demo

demo: build
	./bin/demo

clean:
	rm -rf bin
	rm -rf gen

run-scheduler: build
	./bin/scheduler

run-activator: build
	./bin/activator

docker-build:
	docker build -t k8s-sched-demo-scheduler:latest -f Dockerfile.scheduler .
	docker build -t mock-sandbox:latest -f mock-sandbox/Dockerfile ./mock-sandbox

k8s-deploy: docker-build
	kubectl apply -f k8s/sandbox-namespace.yaml
	kubectl apply -f k8s/redis-deployment.yaml
	kubectl apply -f k8s/scheduler-deployment.yaml
	
k8s-demo: build k8s-deploy
	./demo.sh --client-only 