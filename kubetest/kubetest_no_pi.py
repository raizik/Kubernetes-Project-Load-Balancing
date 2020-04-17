import random
import threading
import time
import urllib.request
import collections
import sys
import subprocess
import os
import signal

ARRIVE_RATE = 16
WORK_RATE = 16
TOTAL_WORKS = 1024**1 # Run simulation until TOTAL_WORKS have been simulated.
TARGET = ""

RESULTSFILE = ""
MODE1 = ""
MODE2 = ""

# After how many seconds since start record pod busy time
CPUStartRecordTime = 2

starttime = 0
endtime = 0

currentwaitsum = 0
requestcount = 0

def run():
    global starttime, endtime, currentwaitsum, requestcount
    threads = []
    procs = []
    #threadsTwo = []
    starttime = time.time()
    for _ in range(TOTAL_WORKS):
        #fill IDLE list
        #threadSec = threading.Thread(group=None, target=threadSecond, name=None, args=(), kwargs={})
        #threadSec.start()
        #threadsTwo.append(threadSec)
        # Decide how much time to wait until new work arrives
        process_getrand = subprocess.Popen('./gettwowjsq.sh',shell=True,preexec_fn=os.setsid)
        procs.append(process_getrand)
        wait = random.expovariate(ARRIVE_RATE)
        time.sleep(wait)
        newThread = threading.Thread(group=None, target=threadMain, name=None, args=(), kwargs={})
        newThread.sim_id = len(threads)
        requestcount += 1
        newThread.start()
        threads.append(newThread)


    print("Done requests, waiting for them to complete")
    endtime = time.time()
    for proc in procs:
        os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
    counter = {}

    for thread in threads:
        thread.join()
  #  for thread in threadsTwo:
   #     thread.join()

    totaltime = (endtime) - (starttime + CPUStartRecordTime)
    sum = 0
    for p in pods.values():
        p.collectBusyTime()
        print("Pod %s percent busy: %.2f%%" % (p.name, p.timebusy / totaltime * 100.0 ))
        sum += p.timebusy
    eff = (sum / totaltime * 100.0 / len(pods))
    print("Average pod busy time: %.2f%%" % eff)


    ###############3
    sum = 0
    httpsum = 0
    worst = 0
    for thread in threads:
        sum += thread.sim_timeWaiting
        httpsum += thread.sim_httptime
        if worst < thread.sim_timeWaiting: worst = thread.sim_timeWaiting

        if thread.sim_givenPod not in counter:
            counter[thread.sim_givenPod] = 1
        else:
            counter[thread.sim_givenPod] += 1


    sum /= len(threads)
    httpsum /= len(threads)

    print("Optimal average wait time: %s, actual time: %s" % (1 / WORK_RATE, sum))
    print("Worst: %s" % worst)

    f = open(RESULTSFILE, "a")
    f.write("%s\t %s\t %s\t %s\t %s%%\t %s\t %s\t %s\n" % (WORK_RATE, ARRIVE_RATE, MODE1, MODE2, eff, sum, httpsum, worst))
    f.close()
    file_temp.write("%s\t %s\n" % (ARRIVE_RATE, eff))



pods = {}
class pod:
    def __init__(self, name):
        self.name = name
        self.timebusy = 0
        self.worktimes = []

    @classmethod
    def getWorker(cls, name):
        if name not in pods:
            pods[name] = cls(name)

        return pods[name]


    def addBusyTime(self, t, podqueue):
        # service time, time service started
        self.worktimes.append((t, time.time() + (podqueue - t)))

    def collectBusyTime(self):
        for item in self.worktimes:
            busytime, workstarttime = item
            workendtime = workstarttime + busytime

            # too early
            if workendtime < starttime + CPUStartRecordTime:
                continue
            # too late
            if workstarttime > endtime:
                continue

            # Cut some of the start
            if workstarttime < starttime + CPUStartRecordTime:
                workstarttime = starttime + CPUStartRecordTime

            # cut some at the end
            if workendtime > endtime:
                workendtime = endtime

            self.timebusy += workendtime - workstarttime



#def threadSecond():
    #subprocess.call(['./idleloop.sh'])

def threadMain():
    global currentwaitsum

    reqstart = time.time()
    data = urllib.request.urlopen(TARGET).read().decode("utf-8")
    reqend = time.time()

    t = threading.currentThread()
    t.sim_givenPod, t.sim_timeWaiting, t.sim_timeInService, t.sim_connectionCountWhenConnected = data.split("\r\n")[:4]
    t.sim_timeInService = float(t.sim_timeInService) / 1000.0
    t.sim_timeWaiting = float(t.sim_timeWaiting) / 1000.0
    t.sim_httptime = reqend - reqstart
    t.sim_connectionCountWhenConnected = int(t.sim_connectionCountWhenConnected)

    p = pod.getWorker(t.sim_givenPod)

    currentwaitsum += t.sim_timeWaiting
    p.addBusyTime(t.sim_timeInService, t.sim_timeWaiting)

    print(("%s:" % t.sim_id).ljust(5) +
          "Pod %s, Load: %s, service time: %.2fs, service with queue: %.2fs, Avg: %.2fs" % (
                                                                               t.sim_givenPod.split("-")[1],
                                                                               t.sim_connectionCountWhenConnected,
                                                                               t.sim_timeInService,
                                                                               t.sim_timeWaiting,
                                                                               currentwaitsum/requestcount
                                                                        ))



def printargs():
    print("required arguments: IP:Port replyRate arriveRate totalworks mode1 mode2 resultsFileName.txt")
    sys.exit(-1)

if __name__ == "__main__":
    if len(sys.argv) < 8: printargs()

    IPPort = sys.argv[1]
    WORK_RATE = float(sys.argv[2])
    ARRIVE_RATE = float(sys.argv[3])
    TOTAL_WORKS = int(sys.argv[4])
    MODE1 = sys.argv[5]
    MODE2 = sys.argv[6]

    file_temp = open("output_sim.csv", "a+")

    RESULTSFILE = sys.argv[7]

    


    TARGET = "http://%s/delay?rate=%s" % (IPPort, WORK_RATE)
    port_temp_array = IPPort.split(":")
    port_temp = port_temp_array[1]
    #print(port_temp)
    #subprocess.Popen(["bash", "pro.sh", "Argument1"])
 #   process_getpodsnamesipport = subprocess.Popen(['./getpodsnamesipport.sh', str(port_temp)],preexec_fn=os.setsid)    
    run()
 #   os.killpg(os.getpgid(process_getpodsnamesipport.pid), signal.SIGKILL)
    file_temp.close()
