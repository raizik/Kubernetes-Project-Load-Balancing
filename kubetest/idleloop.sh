#!/bin/bash


x=1
while [ $x -le 2 ]

do

   kubectl logs --since=1s webr-master --context kind-kind > webr_master_log.txt

   #check if not empty -- this is a case of numIlde == 0 : all pods are busy
   numlinesinlog=$( cat webr_master_log.txt | wc -l )
   if [ ${numlinesinlog} -le 1 ];then
        echo 0 > num_idle.txt
        continue
   fi
   #choose random pod
   echo $( shuf -n 1 webr_master_log.txt ) > random_idle_pod_temp.txt
   # format file to ip:port lines
    echo $(  cat random_idle_pod_temp.txt | grep -o -E '[0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+' )  > random_idle_pod.txt

   num_idle=$( awk '!a[$0]++' webr_master_log.txt | wc -l )
   echo ${num_idle} > num_idle.txt

   x=$(( $x + 1 ))
done
