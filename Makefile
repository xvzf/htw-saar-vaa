NUM_NODES ?= "7"
NUM_EDGES ?= "11"

RUMOR ?= "SomeRumor145678"
RUMOR_C ?= "2"

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

banking: banking-leader-elect

banking-leader-elect:
	go run ./cmd/client/main.go --config="./config.txt" --type="BANKING" --payload="coordinator"

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
