NUM_NODES ?= "6"
NUM_EDGES ?= "10"

startup:
	go run ./cmd/client/main.go --config="./config.txt" --type="CONTROL" --payload="STARTUP"

shutdown:
	go run ./cmd/client/main.go --config="./config.txt" --type="CONTROL" --payload="SHUTDOWN"

gen: gengraph
	jsonnet --ext-str nodeCount=${NUM_NODES} hack/gen-launch.jsonnet | jq -r '."launch.sh"' > launch.sh
	jsonnet --ext-str nodeCount=${NUM_NODES} hack/gen-launch.jsonnet | jq -r '."config.txt"' > config.txt

gengraph:
	go run ./cmd/graphgen/main.go --graph="./graph.txt" --m=${NUM_EDGES} --n=${NUM_NODES} --create

