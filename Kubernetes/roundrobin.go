/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package userspace

import (
	"errors"
	"net"
	"net/http" // TEMP
	//"os"	// Needed for file reading
	"reflect"
	"strconv"
	"sync"
	"time"
	"math/rand"
	"io/ioutil"
	"strings"
	//"log"	// Needed for file reading
	//"encoding/json"
	//"bufio"	// Needed for file reading

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/proxy"
	"k8s.io/kubernetes/pkg/proxy/util"
	"k8s.io/kubernetes/pkg/util/slice"
)

var (
	ErrMissingServiceEntry = errors.New("missing service entry")
	ErrMissingEndpoints    = errors.New("missing endpoints")
)

type affinityState struct {
	clientIP string
	//clientProtocol  api.Protocol //not yet used
	//sessionCookie   string       //not yet used
	endpoint string
	lastUsed time.Time
}

type affinityPolicy struct {
	affinityType v1.ServiceAffinity
	affinityMap  map[string]*affinityState // map client IP -> affinity info
	ttlSeconds   int
}

// LoadBalancerRR is a round-robin load balancer.
type LoadBalancerRR struct {
	lock     sync.RWMutex
	services map[proxy.ServicePortName]*balancerState
}

// Ensure this implements LoadBalancer.
var _ LoadBalancer = &LoadBalancerRR{}


type balancerState struct {
	endpoints []string // a list of "ip:port" style strings
	index     int      // current index into endpoints
	affinity  affinityPolicy
	busyPods map[string]string // busyPods[i] == j <=> endpoint ipport j is busy. init to empty
	last_endpoint string      // last ipport pod the LB sent a job to
	idleTokens int	// number of idle tokens. init to number of endpoints 
}

func newAffinityPolicy(affinityType v1.ServiceAffinity, ttlSeconds int) *affinityPolicy {
	return &affinityPolicy{
		affinityType: affinityType,
		affinityMap:  make(map[string]*affinityState),
		ttlSeconds:   ttlSeconds,
	}
}

// NewLoadBalancerRR returns a new LoadBalancerRR.
func NewLoadBalancerRR() *LoadBalancerRR {
	return &LoadBalancerRR{
		services: map[proxy.ServicePortName]*balancerState{},
}
}

func (lb *LoadBalancerRR) NewService(svcPort proxy.ServicePortName, affinityType v1.ServiceAffinity, ttlSeconds int) error {
	klog.V(4).Infof("LoadBalancerRR NewService %q", svcPort)
	lb.lock.Lock()
	defer lb.lock.Unlock()
	lb.newServiceInternal(svcPort, affinityType, ttlSeconds)
	return nil
}

// This assumes that lb.lock is already held.
func (lb *LoadBalancerRR) newServiceInternal(svcPort proxy.ServicePortName, affinityType v1.ServiceAffinity, ttlSeconds int) *balancerState {
	if ttlSeconds == 0 {
		ttlSeconds = int(v1.DefaultClientIPServiceAffinitySeconds) //default to 3 hours if not specified.  Should 0 be unlimited instead????
	}

	if _, exists := lb.services[svcPort]; !exists {
		lb.services[svcPort] = &balancerState{affinity: *newAffinityPolicy(affinityType, ttlSeconds)}
		klog.V(4).Infof("LoadBalancerRR service %q did not exist, created", svcPort)
	} else if affinityType != "" {
		lb.services[svcPort].affinity.affinityType = affinityType
	}
	return lb.services[svcPort]
}

func (lb *LoadBalancerRR) DeleteService(svcPort proxy.ServicePortName) {
	klog.V(4).Infof("LoadBalancerRR DeleteService %q", svcPort)
	lb.lock.Lock()
	defer lb.lock.Unlock()
	delete(lb.services, svcPort)
}

// return true if this service is using some form of session affinity.
func isSessionAffinity(affinity *affinityPolicy) bool {
	// Should never be empty string, but checking for it to be safe.
	if affinity.affinityType == "" || affinity.affinityType == v1.ServiceAffinityNone {
		return false
	}
	return true
}

// ServiceHasEndpoints checks whether a service entry has endpoints.
func (lb *LoadBalancerRR) ServiceHasEndpoints(svcPort proxy.ServicePortName) bool {
	lb.lock.Lock()
	defer lb.lock.Unlock()
	state, exists := lb.services[svcPort]
	// TODO: while nothing ever assigns nil to the map, *some* of the code using the map
	// checks for it.  The code should all follow the same convention.
	return exists && state != nil && len(state.endpoints) > 0
}

type loadresponse struct {
    candidate string
    load  int
}



func getload(endpoint string) int {
	resp, err := http.Get("http://" + endpoint + "/count")
	if err != nil {
		return -1
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return -2
	}

	res, err := strconv.Atoi(string(body))
	if err != nil {
		return -3
	}
	return res
}

func getloadasync(endpoint string, responseChannel chan loadresponse) {
	res := getload(endpoint)
	responseChannel <- loadresponse{candidate: endpoint, load: res}
}


// NextEndpoint returns a service endpoint.
// The service endpoint is chosen using the round-robin algorithm.
func (lb *LoadBalancerRR) NextEndpoint(svcPort proxy.ServicePortName, srcAddr net.Addr, sessionAffinityReset bool) (string, error) {
	// Coarse locking is simple.  We can get more fine-grained if/when we
	// can prove it matters.
	lb.lock.Lock()
	defer lb.lock.Unlock()

	state, exists := lb.services[svcPort]
	if !exists || state == nil {
		return "", ErrMissingServiceEntry
	}
	if len(state.endpoints) == 0 {
		return "", ErrMissingEndpoints
	}
	klog.V(4).Infof("NextEndpoint for service %q, srcAddr=%v: endpoints: %+v", svcPort, srcAddr, state.endpoints)


	// Temmp workaround for switching mode without restarting.
	typ := 3
	choicesCount := 2
	dat, err := ioutil.ReadFile("/home/iashken/tempconfig")
	if err == nil {
		ascii := string(dat)
		splt := strings.Split(ascii, "\n")


		typ, _ = strconv.Atoi(splt[0])
		if (len(splt) > 1) {
			choicesCount, _ = strconv.Atoi(splt[1])
		}
	}
	// typ := 1
	if (typ == 0) { // round robin
		endpoint := state.endpoints[state.index]
		state.index = (state.index + 1) % len(state.endpoints)
		return endpoint, nil
	} else if (typ == 1) { // choices

		// firstMu := 10
		// secondMu := 10
		if (choicesCount > len(state.endpoints)) {
			choicesCount = len(state.endpoints)
		}

		randsource := rand.NewSource(time.Now().UnixNano())
		randgenerator := rand.New(randsource)
		messages := make(chan loadresponse, choicesCount)

		for j := 0; j < choicesCount; j++ {
			randomnum := randgenerator.Intn(len(state.endpoints))
			getloadasync(state.endpoints[randomnum], messages)
		}

		bestload := loadresponse{candidate: "", load: 100000000};
		for j := 0; j < choicesCount; j++ {
			load := <- messages
			if (load.load >= 0 && load.load < bestload.load) {
				bestload = load;
			}
		}
		return bestload.candidate, nil

	} else if (typ == 2) { // random
		randsource := rand.NewSource(time.Now().UnixNano())
		randgenerator := rand.New(randsource)
		randomnum := randgenerator.Intn(len(state.endpoints))
		return state.endpoints[randomnum], nil
	} else if (typ == 3) { // HTTP gets
		bestload := 10000000
		bestcandidate := 0
		for j := 0; j < len(state.endpoints); j++ {
			currentload := getload(state.endpoints[j])
			if (currentload >= 0 && currentload < bestload) {
				bestcandidate = j
				bestload = currentload
			}
		}
		return state.endpoints[bestcandidate], nil


	} else if (typ == 4) { //async HTTP gets

		messages := make(chan loadresponse, len(state.endpoints))
		for j := 0; j < len(state.endpoints); j++ {
			getloadasync(state.endpoints[j], messages)
		}

		bestload := loadresponse{candidate: "", load: 100000000};
		for j := 0; j < len(state.endpoints); j++ {
			load := <- messages
			if (load.load >= 0 && load.load < bestload.load) {
				bestload = load;
			}
		}
		return bestload.candidate, nil
	} else if (typ == 5) { // NEW Persistent-Idle Load balancer
		candidate := ""
		//read num idle pods
		dat_num_idle, err_idle := ioutil.ReadFile("/home/iashken/num_idle.txt")
		if err_idle != nil {
				panic(err_idle)
		}
		numIdle := 0
		if err_idle == nil {
			ascii_idle := string(dat_num_idle)
			splt_idle := strings.Split(ascii_idle, "\n")
			numIdle, _ = strconv.Atoi(splt_idle[0])
		}
		dat_rand_pod, err_rand_pod := ioutil.ReadFile("/home/iashken/random_idle_pod.txt")
		if err_rand_pod != nil {
				panic(err_rand_pod)
		}
		random_ipport := ""
		if err_rand_pod == nil {
			ascii_rand_pod := string(dat_rand_pod)
			splt_rand_pod := strings.Split(ascii_rand_pod, "\n")
			random_ipport = splt_rand_pod[0]
		}
		//remove current idle endpoint from busyPods
		_, ok_remove := state.busyPods[random_ipport];
    if ok_remove {
        delete(state.busyPods, random_ipport);
    }
		//if lb has idle tokens (there are idle pods)
		if (state.idleTokens > 0) {
			//choose random pod
			//IDLE messages received
			if (random_ipport != "") {
				candidate = random_ipport
			} /*initilization (no IDLE message received yet)*/ else {
				randsource := rand.NewSource(time.Now().UnixNano())
				randgenerator := rand.New(randsource)
				// choose random pod from currently available pods
				randomnum := 0
				do
				{
					randomnum = randgenerator.Intn(len(state.endpoints))
					candidate = state.endpoints[randomnum]
				}
				while (ok := state.busyPods[candidate]; ok)
			}
			//send token to chosen endpoint
			state.idleTokens -= 1
			//mark last ep
			state.last_endpoint = candidate
			//add chosen pod to busy pods map
			state.busyPods[candidate] = candidate
		} /*lb has no idle tokens (all pods busy)*/else if (state.idleTokens == 0) {
			//choose last ep as candidate
			candidate = state.last_endpoint
			//increase by num of incoming idle pods
			state.idleTokens += numIdle
		}
		return candidate, nil
	} else if (typ == 6) { // NEW WJSQ Load balancer

		randsource := rand.NewSource(time.Now().UnixNano())
		randgenerator := rand.New(randsource)
		firstLoc := randgenerator.Intn(len(state.endpoints))
		secondLoc := randgenerator.Intn(len(state.endpoints))
		candidate1 := ""
		candidate2 := ""
		firstMu := 10
		secondMu := 10
/*
splt := strings.Split(ascii, "\n")


typ, _ = strconv.Atoi(splt[0])
*/
		dat_f, err_f := ioutil.ReadFile("/home/iashken/first_random_pod.txt")
					if err_f == nil {
									ascii_f := string(dat_f)
									splt_f := strings.Split(ascii_f, "\x20")
									candidate1 = splt_f[0]
									first_loc := strings.Split(splt_f[1], "\n")
									firstLoc, _ = strconv.Atoi(first_loc[0])
					}
	dat_s, err_s := ioutil.ReadFile("/home/iashken/second_random_pod.txt")
									if err_s == nil {
													ascii_s := string(dat_s)
													splt_s := strings.Split(ascii_s, "\x20")
													candidate2 = splt_s[0]
													second_loc := strings.Split(splt_s[1], "\n")
													secondLoc, _ = strconv.Atoi(second_loc[0])
									}

		//candidate1 = state.sortedEndpoints[firstLoc]
		//candidate1 = state.endpoints[j]
		switch choicesCount {
		case 0:
			if (firstLoc>=5){
				firstMu = 50.0
			}
		case 1:
			if (firstLoc>=1){
				firstMu = 50.0
			}
		case 2:
			if (firstLoc==9){
				firstMu = 50.0
			}
		}
		//candidate2 = state.sortedEndpoints[secondLoc]
		//candidate2 = state.endpoints[secondLoc]
		switch choicesCount {
		case 0:
			if (secondLoc>=5){
				secondMu = 50.0
			}
		case 1:
			if (secondLoc>=1){
				secondMu = 50.0
			}
		case 2:
			if (secondLoc==9){
				secondMu = 50.0
			}
		}
			// bestcanditate := ""
		messages := make(chan loadresponse, 2)
		getloadasync(candidate1, messages)
		getloadasync(candidate2, messages)
		bestload := loadresponse{candidate: "", load: 100000000};
		for j := 0; j < 2; j++ {
			load := <- messages
			switch load.candidate {
			case candidate1:
				if (load.load >= 0 && (load.load / firstMu) < (bestload.load / secondMu)) {
					bestload = load;
				}
			case candidate2:
				if (load.load >= 0 && (load.load / secondMu) < (bestload.load / firstMu)) {
					bestload = load;
				}
			}
		}
		return bestload.candidate, nil
	}
	return "", ErrMissingEndpoints
}

type hostPortPair struct {
	host string
	port int
}

func isValidEndpoint(hpp *hostPortPair) bool {
	return hpp.host != "" && hpp.port > 0
}

func flattenValidEndpoints(endpoints []hostPortPair) []string {
	// Convert Endpoint objects into strings for easier use later.  Ignore
	// the protocol field - we'll get that from the Service objects.
	var result []string
	for i := range endpoints {
		hpp := &endpoints[i]
		if isValidEndpoint(hpp) {
			result = append(result, net.JoinHostPort(hpp.host, strconv.Itoa(hpp.port)))
		}
	}
	return result
}

// Remove any session affinity records associated to a particular endpoint (for example when a pod goes down).
func removeSessionAffinityByEndpoint(state *balancerState, svcPort proxy.ServicePortName, endpoint string) {
	for _, affinity := range state.affinity.affinityMap {
		if affinity.endpoint == endpoint {
			klog.V(4).Infof("Removing client: %s from affinityMap for service %q", affinity.endpoint, svcPort)
			delete(state.affinity.affinityMap, affinity.clientIP)
		}
	}
}

// Loop through the valid endpoints and then the endpoints associated with the Load Balancer.
// Then remove any session affinity records that are not in both lists.
// This assumes the lb.lock is held.
func (lb *LoadBalancerRR) updateAffinityMap(svcPort proxy.ServicePortName, newEndpoints []string) {
	allEndpoints := map[string]int{}
	for _, newEndpoint := range newEndpoints {
		allEndpoints[newEndpoint] = 1
	}
	state, exists := lb.services[svcPort]
	if !exists {
		return
	}
	for _, existingEndpoint := range state.endpoints {
		allEndpoints[existingEndpoint] = allEndpoints[existingEndpoint] + 1
	}
	for mKey, mVal := range allEndpoints {
		if mVal == 1 {
			klog.V(2).Infof("Delete endpoint %s for service %q", mKey, svcPort)
			removeSessionAffinityByEndpoint(state, svcPort, mKey)
		}
	}
}

// buildPortsToEndpointsMap builds a map of portname -> all ip:ports for that
// portname. Expode Endpoints.Subsets[*] into this structure.
func buildPortsToEndpointsMap(endpoints *v1.Endpoints) map[string][]hostPortPair {
	portsToEndpoints := map[string][]hostPortPair{}
	for i := range endpoints.Subsets {
		ss := &endpoints.Subsets[i]
		for i := range ss.Ports {
			port := &ss.Ports[i]
			for i := range ss.Addresses {
				addr := &ss.Addresses[i]
				portsToEndpoints[port.Name] = append(portsToEndpoints[port.Name], hostPortPair{addr.IP, int(port.Port)})
				// Ignore the protocol field - we'll get that from the Service objects.
			}
		}
	}
	return portsToEndpoints
}

func (lb *LoadBalancerRR) OnEndpointsAdd(endpoints *v1.Endpoints) {
	portsToEndpoints := buildPortsToEndpointsMap(endpoints)

	lb.lock.Lock()
	defer lb.lock.Unlock()

	for portname := range portsToEndpoints {
		svcPort := proxy.ServicePortName{NamespacedName: types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}, Port: portname}
		newEndpoints := flattenValidEndpoints(portsToEndpoints[portname])
		state, exists := lb.services[svcPort]

		if !exists || state == nil || len(newEndpoints) > 0 {
			klog.V(1).Infof("LoadBalancerRR: Setting endpoints for %s to %+v", svcPort, newEndpoints)
			lb.updateAffinityMap(svcPort, newEndpoints)
			// OnEndpointsAdd can be called without NewService being called externally.
			// To be safe we will call it here.  A new service will only be created
			// if one does not already exist.  The affinity will be updated
			// later, once NewService is called.
			state = lb.newServiceInternal(svcPort, v1.ServiceAffinity(""), 0)
			state.endpoints = util.ShuffleStrings(newEndpoints)

			/*state.sortedEndpoints = make([]string, len(state.endpoints)) // Ziv's addition
			file, err := os.Open("/home/iashken/pods_array.txt")
			if err != nil {
				log.Fatalf("failed opening file: %s", err)
			}
			scanner := bufio.NewScanner(file)
			scanner.Split(bufio.ScanLines)
			var txtlines []string
			for scanner.Scan() {
				txtlines = append(txtlines, scanner.Text())
			}
			file.Close()
			i := 0
			//const_port := ":8080"
			for _, eachline := range txtlines {
				state.sortedEndpoints[i] = eachline
				i = i + 1
			}*/


			// Reset the round-robin index.
			state.index = 0
			//reset num of idle tokens
			state.idleTokens = len(state.endpoints)
			//reset busyPods
			state.busyPods = make(map[string]int32, len(state.endpoints))
		}
	}
}

func (lb *LoadBalancerRR) OnEndpointsUpdate(oldEndpoints, endpoints *v1.Endpoints) {
	portsToEndpoints := buildPortsToEndpointsMap(endpoints)
	oldPortsToEndpoints := buildPortsToEndpointsMap(oldEndpoints)
	registeredEndpoints := make(map[proxy.ServicePortName]bool)

	lb.lock.Lock()
	defer lb.lock.Unlock()

	for portname := range portsToEndpoints {
		svcPort := proxy.ServicePortName{NamespacedName: types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}, Port: portname}
		newEndpoints := flattenValidEndpoints(portsToEndpoints[portname])
		state, exists := lb.services[svcPort]

		curEndpoints := []string{}
		if state != nil {
			curEndpoints = state.endpoints
		}

		if !exists || state == nil || len(curEndpoints) != len(newEndpoints) || !slicesEquiv(slice.CopyStrings(curEndpoints), newEndpoints) {
			klog.V(1).Infof("LoadBalancerRR: Setting endpoints for %s to %+v", svcPort, newEndpoints)
			lb.updateAffinityMap(svcPort, newEndpoints)
			// OnEndpointsUpdate can be called without NewService being called externally.
			// To be safe we will call it here.  A new service will only be created
			// if one does not already exist.  The affinity will be updated
			// later, once NewService is called.
			state = lb.newServiceInternal(svcPort, v1.ServiceAffinity(""), 0)
			state.endpoints = util.ShuffleStrings(newEndpoints)

		/*	state.sortedEndpoints = make([]string, len(state.endpoints)) // Ziv's addition
			file, err := os.Open("/home/iashken/pods_array.txt")
			if err != nil {
				log.Fatalf("failed opening file: %s", err)
			}
			scanner := bufio.NewScanner(file)
			scanner.Split(bufio.ScanLines)
			var txtlines []string
			for scanner.Scan() {
				txtlines = append(txtlines, scanner.Text())
			}
			file.Close()
			i := 0
			//const_port := ":8080"
			for _, eachline := range txtlines {
				state.sortedEndpoints[i] = eachline
				i = i + 1
			}*/

			// Reset the round-robin index.
			state.index = 0
			//reset num of idle tokens
			state.idleTokens = len(state.endpoints)
			//reset busyPods
			state.busyPods = make(map[string]int32, len(state.endpoints))
		}
		registeredEndpoints[svcPort] = true
	}

	// Now remove all endpoints missing from the update.
	for portname := range oldPortsToEndpoints {
		svcPort := proxy.ServicePortName{NamespacedName: types.NamespacedName{Namespace: oldEndpoints.Namespace, Name: oldEndpoints.Name}, Port: portname}
		if _, exists := registeredEndpoints[svcPort]; !exists {
			lb.resetService(svcPort)
		}
	}
}

func (lb *LoadBalancerRR) resetService(svcPort proxy.ServicePortName) {
	// If the service is still around, reset but don't delete.
	if state, ok := lb.services[svcPort]; ok {
		if len(state.endpoints) > 0 {
			klog.V(2).Infof("LoadBalancerRR: Removing endpoints for %s", svcPort)
			state.endpoints = []string{}
		}
		state.index = 0
		//reset num of idle tokens
		state.idleTokens = len(state.endpoints)
		//reset busyPods
		state.busyPods = make(map[string]int32, len(state.endpoints))
		state.affinity.affinityMap = map[string]*affinityState{}
	}
}

func (lb *LoadBalancerRR) OnEndpointsDelete(endpoints *v1.Endpoints) {
	portsToEndpoints := buildPortsToEndpointsMap(endpoints)

	lb.lock.Lock()
	defer lb.lock.Unlock()

	for portname := range portsToEndpoints {
		svcPort := proxy.ServicePortName{NamespacedName: types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}, Port: portname}
		lb.resetService(svcPort)
	}
}

func (lb *LoadBalancerRR) OnEndpointsSynced() {
}

// Tests whether two slices are equivalent.  This sorts both slices in-place.
func slicesEquiv(lhs, rhs []string) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	if reflect.DeepEqual(slice.SortStrings(lhs), slice.SortStrings(rhs)) {
		return true
	}
	return false
}

func (lb *LoadBalancerRR) CleanupStaleStickySessions(svcPort proxy.ServicePortName) {
	lb.lock.Lock()
	defer lb.lock.Unlock()

	state, exists := lb.services[svcPort]
	if !exists {
		return
	}
	for ip, affinity := range state.affinity.affinityMap {
		if int(time.Since(affinity.lastUsed).Seconds()) >= state.affinity.ttlSeconds {
			klog.V(4).Infof("Removing client %s from affinityMap for service %q", affinity.clientIP, svcPort)
			delete(state.affinity.affinityMap, ip)
		}
	}
}
