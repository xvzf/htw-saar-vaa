import os
import time

rumor = "SOME_MARKER_123456"
nodes =  [6, 8, 10, 12, 14, 16, 18, 20, 22, 24]
cs = [2, 3]

# Generate graphs
for n in nodes:
    m = n + int(n/2)
    print(f"[+] Generating graph for {n} nodes, {m} edges")
    os.system(f"go run ../../cmd/graphgen/main.go --graph=./lib/assets/{n}-{m}.graph.txt --m={m} --n={n} --create")

# Generate assets
with open("./lib/assets/main.jsonnet", "w") as f:
    print("[+] Generating jsonnet import")
    f.write("{\n")
    f.write(f"  'maxN': {max(nodes)},\n")
    for n in nodes:
        m = n + int(n/2)
        name = f"{n}-{m}"
        f.write(f"  '{name}': (importstr './{name}.graph.txt'),\n")
    f.write("}")

# Run scenarios :)
for n in nodes:
    m = n + int(n/2)
    for c in cs:
        # Deploy environmet
        print(f"[+] Deploy scenario nodes: {n}, edges: {m}, c: {c}")
        os.system(f"tk apply environments/rumor --tla-str scenario='{n}-{m}-{c}' --dangerous-auto-approve")
        # Wait for all pods to be up
        print("[+] Wait for pods to be up")
        os.system("kubectl wait -n default --for=condition=Ready pod --all")
        time.sleep(10)
        print("[+] Trigger rumor")
        os.system(f"kubectl exec -it node-{n}-{m}-{c}-1 -- /client --type='CONTROL' --payload='DISTRIBUTE RUMOR {c};{rumor}'")
        print("[+] Wait 30s")
        time.sleep(30)
        print("[+] Teardown")
        os.system("kubectl delete -n default pod --all --grace-period=0")
