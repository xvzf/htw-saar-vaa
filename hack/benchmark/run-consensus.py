import os
import time

# Deploy environmet
for i in range(10):
    print(f"[+] Deploy test{i}")
    os.system(f"tk apply environments/consensus --dangerous-auto-approve --name=test{i}")

print("[+] Wait for pods to be up")
os.system("kubectl wait -n default --for=condition=Ready pod --all")


print("[+] Wait 30s")
time.sleep(30)
os.system("sh start-consensus-parallel.sh")
# print("[+] Wait 30s")
# time.sleep(30)
# print("[+] Teardown")
# os.system("kubectl delete -n default pod --all --grace-period=0")
