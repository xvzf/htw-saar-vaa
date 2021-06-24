{
  local nodeCount = std.parseInt(std.extVar("nodeCount")),
  local graphPath = "./graph.txt",

  // Construct config.txt
  'config.txt': std.join("\n", [
    "%(uid)d 127.0.0.1:%(port)d" % {uid: u, port: 4000 + u},
    for u in std.range(1, nodeCount)
  ]),

  // Launch script, launching each node in a different tmux pane
  'launch.sh':
  // Launch new tmux session
  |||
    #!/bin/sh -eux

    SESSION="vaa"

    tmux new-session -d -s $SESSION

    # Name first Window and start zsh
    tmux rename-window -t 0 'control'
    tmux send-keys -t 'control' 'zsh' C-m 'clear' C-m
  ||| + std.join("\n", [
    // Tmux pane for each node
    |||
      tmux new-window -t $SESSION:%(uid)s -n 'node-%(uid)d'
      tmux send-keys -t 'node-%(uid)d' 'go run ./cmd/node/main.go --uid=%(uid)d --config="./config.txt" --graph="./graph.txt" --metric=":%(metricPort)d" --consensus-m=4 --consensus-amax=20 --consensus-p=3 --consensus-s=4' C-m
    ||| % {uid: u, metricPort: 5000 + u},
    for u in std.range(1, nodeCount)
  ]) + |||
    # Attach to control panel
    tmux attach-session -t $SESSION:0
  |||,

}
