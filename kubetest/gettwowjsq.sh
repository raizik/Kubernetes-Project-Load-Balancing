#!/bin/bash

first_pod=$( shuf -n 1 pods_array.txt )
first_pod_id=$(($( grep -n "${first_pod}" pods_array.txt  | head -n 1 | cut -d: -f1 ) - 1))
sec_pod=$( shuf -n 1 pods_array.txt )
sec_pod_id=$(($( grep -n "${sec_pod}" pods_array.txt | head -n 1 | cut -d: -f1 ) - 1))
echo "${first_pod} ${first_pod_id}" > first_random_pod.txt
echo "${sec_pod} ${sec_pod_id}" > second_random_pod.txt
#echo ${first_pod_id}
