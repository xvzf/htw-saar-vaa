#!/bin/sh -eux

SESSION="vaa"

tmux new-session -d -s $SESSION

# Name first Window and start zsh
tmux rename-window -t 0 'control'
tmux send-keys -t 'control' 'zsh' C-m 'clear' C-m
tmux new-window -t $SESSION:1 -n 'node-1'
tmux send-keys -t 'node-1' 'go run ./cmd/node/main.go --uid=1 --config="./config.txt" --graph="./graph.txt" --metric=":5001"' C-m

tmux new-window -t $SESSION:2 -n 'node-2'
tmux send-keys -t 'node-2' 'go run ./cmd/node/main.go --uid=2 --config="./config.txt" --graph="./graph.txt" --metric=":5002"' C-m

tmux new-window -t $SESSION:3 -n 'node-3'
tmux send-keys -t 'node-3' 'go run ./cmd/node/main.go --uid=3 --config="./config.txt" --graph="./graph.txt" --metric=":5003"' C-m

tmux new-window -t $SESSION:4 -n 'node-4'
tmux send-keys -t 'node-4' 'go run ./cmd/node/main.go --uid=4 --config="./config.txt" --graph="./graph.txt" --metric=":5004"' C-m

tmux new-window -t $SESSION:5 -n 'node-5'
tmux send-keys -t 'node-5' 'go run ./cmd/node/main.go --uid=5 --config="./config.txt" --graph="./graph.txt" --metric=":5005"' C-m

tmux new-window -t $SESSION:6 -n 'node-6'
tmux send-keys -t 'node-6' 'go run ./cmd/node/main.go --uid=6 --config="./config.txt" --graph="./graph.txt" --metric=":5006"' C-m
# Attach to control panel
tmux attach-session -t $SESSION:0

