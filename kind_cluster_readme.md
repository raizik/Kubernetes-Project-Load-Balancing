# Kind

**INSTALL:**

Use instructions from:
https://kind.sigs.k8s.io/


**SET UP:**

Run cluster_create.sh script found in the main folder. It installs everything automatically. In case you want to create a cluster that doesn't contain all the sets of ratios, change cluster_create.sh accordingly (leave only the names of the service and image you'd like to test).

Alternatively, you can use the manual set-up below:

**Setting environment variables:**

    export PATH="$PATH:/home/iashken/kind/bin"
    export PATH="$PATH:/home/iashken/go/bin"
    export GOPATH="/home/iashken/go"

**Setting Docker images**

```docker build -t my-custom-image ./my-image-dir```

Example: 

building the (9 fast:1 slow) ratio image:

```docker build -t 9strong .```

Setting the idle receiver pod for PI:

```docker build -t idlereceiver .```

**Creating the cluster:**

1. run creation command using yaml file found in yamls folder
  ```kind create cluster --config kind_cluster_one_node.yaml```   
2. run context command for setting kubectl to kind context
  ```kubectl cluster-info --context kind-kind```        
3. run the following to load Docker images into your cluster nodes
  ```kind load docker-image my-custom-image```          
  where my-custom-image is the name of the image set when building. In the example: 9strong
                
4. Statefulset creation:
                              ```kubectl apply -f statefulset.yaml --context kind-kind```
where statefulset.yaml is the yaml file in the yamls folder with the same name as the docker image. In the example: stateful_ninestrong.yaml (or stateful_ninestrong_pi.yaml for PI).
5. For PI set idlereceiver image do the following steps:
* ```kind load docker-image idlereceiver```
* ```kubectl apply -f pod.yaml --context kind-kind```


# Running kubetest.py:
 
 
**Getting the master node IP address:**

```kubectl get nodes -o wide --context kind-kind | grep master```


**Getting the service PORT:**

```kubectl get svc -o wide --context kind-kind```

Run .py with **IP:PORT**

**The parameters to kubetest are:**

[IP:PORT] mu(service rate) lambda(arrival rate) num_jobs mode1 mode2 result_file

**Remark:** service rate parameter is meaningful only when running a homogeneous environment This set up guide is for heterogeneous environment only in which case send 0 or any other constant since the service rate for slow pods is 10 and for fast pods 50 and this is set by the Docker images

**[IP:PORT]**

The IP:PORT pair found in the explanation above (IP of master node, PORT of service chosen)

**MODE1:**

0  round robin

1  choices (JSQ)

2  random choice

3  http gets - Synchronuous

4  http gets Asynchronuous

5  Persistent-Idle

6  WJSQ-2

**MODE2:**

*For PI:*

Meaningless (send 0 or any constant)

*For WJSQ-2:*

0 5:5 ratio

1 9:1 (fast:slow) ratio

2 1:9 (fast:slow) ratio

*For JSQ (choices):*

d any number (for example: for JSQ-2 send 2)

## Example:

**Running PI:**

```python3 kubetest_pi.py [IP:PORT] 50 30 1000 5 2 res_pi.txt```

**Running WJSQ-2:**

```python3 kubetest_no_pi.py [IP:PORT] 50 30 1000 6 2 res_wjsq.txt```


# Kubectl useful commands for debugging the cluster

**connecting to a pod as a server with its own fs (log files, etc):**

```kubectl exec -it [pod name] --context kind-kind -- /bin/sh```

**Getting log file of pod:**

```kubectl logs -f [pod name] --context kind-kind```

**Get Cluster log file to one file:**

```kubectl cluster-info dump --context kind-kind```

# Miscellaneous

**Export logs and kubeconfig files to randomly generated folder:**

```kind export kubeconfig --context kind-kind```

```kind export logs --context kind-kind```

*Remark: path is printed to terminal after running*


**Solution to connection loss with kind cluster after reboot:**

Simply run cluster_create.sh



