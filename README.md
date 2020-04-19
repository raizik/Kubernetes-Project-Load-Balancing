# Kubernetes Load Balancing algorithms
In this project I implemented and tested both an improved version of Power-of-2-Choice implemented in the previous semester which is based on weighted queues and a newly researched algorithm - Persistent-Idle (PI). 

I adjusted and compiled the load balancer (kube-proxy) to have the new algorithm and be able to switch between the algorithms at runtime. The Kube code can be found in the kubernetes folder.

I used a cluster comprising of master node and worker node using the tool Kind (more information and installation can be found in kind_cluster_readme.md). 

In order to test the algorithms' performance, I built a Node.js web application that runs on each pod handling HTTP requests sent from a python script (kubetest_pi.py for PI and kubetest_no_pi.py for the rest can be found in kubetest folder) which sends concurrent HTTP Get requests to the Load Balancer. 

I found that PI outperforms all tested algorithms in a heterogeneous environment which I defined by setting different service rates to different pods in the set. This was tested with a set of 10 pods and 3 different sets of fast:weak ratios: 1:9, 9:1 and 5:5.

## Weighted Join-the-Shortest-Queue-2 (JSQ-2)
This algorithm chooses the next pod to send the incoming job to by choosing two pods at random and selecting the pod with the minimal weight which is comprised of the quotient of the pod's load over its' service rate. The current load of a pod is calculated by sending an asynchronous HTTP request to the pod from the Load Balancer’s logic. The bash script for choosing two pods at random gettwowjsq can be found in the kubetest folder 

## Persistent-Idle
This algorithm is based on IDLE messages sent from the pods to the Load Balancer without it having to query them for their availability, service rate, load, etc.. As long as there are free pods - the incoming job is sent to one of them at random. Once there is an incoming job and non of the pods is available - the job would be sent to the last pod that received a job. 
I implemented this by using a Node.js web application running on a pod set in the master node handling HTTP Post requests sent from the pods in the worker node. The dockerimage folder can be found in dockerimages folder. The bash script for choosing random idle pod idleloop can be found in the kubetest folder

