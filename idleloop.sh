#!/bin/bash

x=1
while [ $x -le 3 ]
#while true
do


   timeout 1s kubectl logs -f webr-master --context kind-kind > webr_master_log_new.txt
   # calculate magic_num: magic_num is the size of current "tickets array"
   num_lines_prev=$( cat webr_master_log.txt | wc -l )
   num_lines_curr=$( cat webr_master_log_new.txt | wc -l )
   magic_num=$((${num_lines_curr} - ${num_lines_prev}))
   #TODO: maybe cat only diff'd lines
   # get file log w/o 1st line ("listening on port 8080...")
   tail -n +2 webr_master_log_new.txt > webr_master_log_new_tmp.txt && cp webr_master_log_new_tmp.txt webr_master_log_new.txt
   #tail -n +2 webr_master_log_new.txt > webr_master_log.txt
   #take magic_num last lines of log file
   cat webr_master_log_new.txt > webr_master_log.txt
   if ((${magic_num} > 0));then
        tail -n +${magic_num} webr_master_log.txt > webr_master_log_tmp.txt && cp webr_master_log_tmp.txt webr_master_log.txt
   fi
   #check if not empty -- this is a case of numIlde == 0 : all pods are busy
   if [ ! -s webr_master_log.txt ] || [ ${magic_num} -le 0 ];then
        echo 0 > num_idle.txt
        continue
   fi
   #choose random pod
   echo $( shuf -n 1 webr_master_log.txt ) > random_idle_pod_temp.txt
   # format file to ip:port lines and append to idle_pod_list.txt
   #while IFS= read -r line
   #do
    echo $(  cat random_idle_pod_temp.txt | grep -o -E '[0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+' )  > random_idle_pod.txt
   #done < webr_master_log.txt #done < input_file.txt
   #num_lines=$( cat idle_pod_list.txt | wc -l )
   # get last num lines of file and make the file have non-duplicates
   #TODO: what should magic_num be? 90? 50? 20? 10? maybe decided in advance as num of pods? -> no, needs to be bigger
   #timeout 0.09s gives <=100 lines
   #magic_num=
   #num_last_lines=${magic_num}
   #tail -${num_last_lines} idle_pod_list.txt > idle_pod_list.txt
   #uniq webr_master_log.txt > webr_master_log.txt
   #this is numOfTokens / state.numIdle
   num_idle=$( sort webr_master_log.txt | uniq -u | wc -l )
   echo ${num_idle} > num_idle.txt

   #decrease num_idle by 1
   #echo $((${num_idle} - 1)) > num_idle.txt
   #last_line=$( tail -n 1  webr_master_log.txt )
   #echo ${last_line} | grep -o -E '[0-9]+.[0-9]+.[0-9]+.[0-9]+:[0-9]+'  >> idle_pod_list.txt
   #last_line_two=$( tail -n 1 idle_pod_list.txt )
   #echo ${last_line_two} > idle_pod.txt
   x=$(( $x + 1 ))
done
