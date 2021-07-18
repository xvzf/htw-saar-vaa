PODS=$(kubectl get pods -n default -o name | grep -i ".*-1$")

for pod in $PODS; do
  kubectl exec -n default -it $pod -- /client --config="/config/nodes.txt" --type="CONSENSUS" --payload="coordinator" &
done

wait $(jobs -rp)
