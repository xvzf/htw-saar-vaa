NUM_NODES ?= "10"
NUM_EDGES ?= "16"

RUMOR ?= "SomeRumor12345678"
RUMOR_C ?= "3"

DOCKER_IMAGE = "quay.io/xvzf/vaa:latest"

launch: gen
	sh launch.sh

startup:
	go run ./cmd/client/main.go --config="./config.txt" --type="CONTROL" --payload="STARTUP"

rumor:
	go run ./cmd/client/main.go --connect="[::1]:4006" --type="CONTROL" --payload="DISTRIBUTE RUMOR ${RUMOR_C};${RUMOR}"

consensus: consensus-leader-elect

consensus-leader-elect:
	go run ./cmd/client/main.go --config="./config.txt" --type="CONSENSUS" --payload="coordinator"

shutdown:
	go run ./cmd/client/main.go --config="./config.txt" --type="CONTROL" --payload="SHUTDOWN"

gen: gengraph
	jsonnet --ext-str nodeCount=${NUM_NODES} hack/gen-launch.jsonnet | jq -r '."launch.sh"' > launch.sh
	jsonnet --ext-str nodeCount=${NUM_NODES} hack/gen-launch.jsonnet | jq -r '."config.txt"' > config.txt

gengraph:
	go run ./cmd/graphgen/main.go --graph="./graph.txt" --m=${NUM_EDGES} --n=${NUM_NODES} --create

docker-build:
	docker build . -t ${DOCKER_IMAGE}

docker-push:
	docker push ${DOCKER_IMAGE}
