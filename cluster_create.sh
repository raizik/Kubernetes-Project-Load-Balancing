#!/bin/bash

export PATH="$PATH:/home/iashken/kind/bin"
kind delete cluster kind
kind create cluster --config kind_cluster_one_node.yaml  --name kind
kubectl cluster-info --context kind-kind
docker build -t 9strong .
kind load docker-image 9strong --name kind
docker build -t 9weak .
kind load docker-image 9weak --name kind
docker build -t 9strongpi .
kind load docker-image 9strongpi --name kind
docker build -t 9weakpi .
kind load docker-image 9weakpi --name kind
docker build -t five .
kind load docker-image five --name kind
docker build -t fivepi .
kind load docker-image fivepi --name kind
docker build -t idlereceiver .
kind load docker-image idlereceiver --name kind
kubectl apply -f statefulset_five.yaml --context kind-kind
kubectl apply -f statefulset_five_pi.yaml --context kind-kind
kubectl apply -f statefulset_ninestrong.yaml --context kind-kind
kubectl apply -f statefulset_ninestrong_pi.yaml --context kind-kind
kubectl apply -f statefulset_nineweak.yaml --context kind-kind
kubectl apply -f statefulset_nineweak_pi.yaml --context kind-kind
kubectl apply -f pod.yaml --context kind-kind
