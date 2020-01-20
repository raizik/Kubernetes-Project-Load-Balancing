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
	//"os"
	"reflect"
	"strconv"
	"sync"
	"time"
	"math/rand"
	"io/ioutil"
	"strings"
	//"log"
	//"encoding/json"

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
	// Our addition: 1. a list of "ip:port" strings of IDLE endpoints.
	//							 2. a string of the "ip:port" of the last endpoint a job was sent to.
	//tickets		map[string]int	// tickets[i] == 1 <=> pod i is idle
	//idlePods map[int]string // idlePods[i] == ipport <=> pod ipport is the ith pod to send IDLE
	last_endpoint string      // last ipport pod the LB sent a job to
	numBusy int	//number of idle pods in idlePods array (and number of ones in tickets)
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
    canditate string
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
	responseChannel <- loadresponse{canditate: endpoint, load: res}
}

//racheli 6120
// Start of our added Handler function.
/*
type idleMessage struct{
	msg string
}

type idleHandler struct{
	lb LoadBalancer
	svcPort proxy.ServicePortName
}

func GetIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

func (ih *idleHandler) PIhandler(w http.ResponseWriter, r *http.Request) {
	var data idleMessage

	err := json.NewDecoder(r.Body).Decode(&data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    //racheli 2120
    e := reflect.ValueOf(ih.lb).Elem()
    varValueServices := e.FieldByName("services").Interface().(map[proxy.ServicePortName]*balancerState)

	if data.msg == "IDLE" {
		ipport := GetIP(r)
		new_index := 0
		//racheli 02012020
		for i, point := range (varValueServices[ih.svcPort]).endpoints {
			if point == ipport {
				new_index = i
				break
			}
		}
		//racheli 02012020
		(varValueServices[ih.svcPort]).tickets[new_index] = 1
	}
}
// End of our added Handler function.
*/

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

		bestload := loadresponse{canditate: "", load: 100000000};
		for j := 0; j < choicesCount; j++ {
			load := <- messages
			if (load.load >= 0 && load.load < bestload.load) {
				bestload = load;
			}
		}
		return bestload.canditate, nil

	} else if (typ == 2) { // random
		randsource := rand.NewSource(time.Now().UnixNano())
		randgenerator := rand.New(randsource)
		randomnum := randgenerator.Intn(len(state.endpoints))
		return state.endpoints[randomnum], nil
	} else if (typ == 3) { // HTTP gets
		bestload := 10000000
		bestcanditate := 0
		for j := 0; j < len(state.endpoints); j++ {
			currentload := getload(state.endpoints[j])
			if (currentload >= 0 && currentload < bestload) {
				bestcanditate = j
				bestload = currentload
			}
		}
		return state.endpoints[bestcanditate], nil


	} else if (typ == 4) { //async HTTP gets

		messages := make(chan loadresponse, len(state.endpoints))
		for j := 0; j < len(state.endpoints); j++ {
			getloadasync(state.endpoints[j], messages)
		}

		bestload := loadresponse{canditate: "", load: 100000000};
		for j := 0; j < len(state.endpoints); j++ {
			load := <- messages
			if (load.load >= 0 && load.load < bestload.load) {
				bestload = load;
			}
		}
		return bestload.canditate, nil
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
		//num idle > 0 		=>		there are idle pods
		if (state.numBusy < len(state.endpoints)) {
			//choose random pod
			//IDLE messages received
			if (random_ipport != "") {
				candidate = random_ipport
			} /*init (no IDLE message received yet)*/ else {
				candidate = state.endpoints[state.numBusy]
			}
			//increase num busy pods
			state.numBusy += 1
			//mark last ep
			state.last_endpoint = candidate
		} /*num idle=0=>no idle*/else if (state.numBusy >= len(state.endpoints)) {
			//choose last ep as candidate
			candidate = state.last_endpoint
			//increase numBusy and decrease by num of idle pods
			state.numBusy += 1
			state.numBusy -= numIdle
			if (state.numBusy < 0) {
				state.numBusy = 0
			}
		}
///////////////////////////////////////////////////////////////////////////////////////////

/*		if (numOfTokens > 0) {
      dat_rand_pod, err_rand_pod := ioutil.ReadFile("/home/iashken/random_idle_pod.txt")
  		if err_rand_pod != nil {
          panic(err_rand_pod)
      }
      if err_rand_pod == nil {
        ascii_rand_pod := string(dat_rand_pod)
        splt_rand_pod := strings.Split(ascii_rand_pod, "\n")
        random_ipport := splt_rand_pod[0]
        //random_ipport := ascii_rand_pod
			  candidate = random_ipport
			  state.last_endpoint = random_ipport
        //subtract 1 from num_idle.txt
        message := []byte(strconv.Itoa(numOfTokens - 1))
	      err_rewrite_idlenum := ioutil.WriteFile("/home/iashken/num_idle.txt", message, 0644)
	      if err_rewrite_idlenum != nil {
		     panic(err_rewrite_idlenum)
      	}
			  //state.numIdle -= 1
			/*for i , ticket := range state.tickets{
				if ticket == 0 {
					continue
				}
				if randomnum == 0 {
					canditate = state.endpoints[i]
					new_tickets_i = i
					break
				}
				randomnum = randomnum - 1
			}
			state.last_endpoint = new_tickets_i
			state.tickets[new_tickets_i] = 0*/
			//return candidate, nil
      //"/home/iashken/debug_16120.txt"
		//}
    //}
    /*dat_num_idle2, err_idle2 := ioutil.ReadFile("/home/iashken/num_idle.txt")
    if err_idle2 != nil {
        panic(err_idle2)
    }
    numOfTokens2 := 0
    if err_idle2 == nil {
      ascii_idle2 := string(dat_num_idle2)
      splt_idle2 := strings.Split(ascii_idle2, "\n")
      numOfTokens2, _ = strconv.Atoi(splt_idle2[0])
    }
    if (numOfTokens2 == 0) {
      message_debug := []byte("numOfTokens == 0")
      err_debug := ioutil.WriteFile("/home/iashken/debug_16120.txt", message_debug, 0644)
      if err_debug != nil {
       panic(err_debug)
      }

			/*for ep_index_ , ep_ := range state.endpoints{
				if ep_ == state.last_endpoint {
					candidate = state.endpoints[ep_index_]
					break
				}
			}*/
			//debug 16120

			//candidate = state.last_endpoint

		//}
		return candidate, nil
	} else if (typ == 6) { // NEW WJSQ Load balancer
				var distributed [10]float64
		// distributed[0] = (5/14) * 1.0
		for i := 0; i < 5; i++ {
			distributed[i] = (1/30) * 1.0
			distributed[i+5] = (5/30) * 1.0
		}

		randsource := rand.NewSource(time.Now().UnixNano())
		randgenerator := rand.New(randsource)
		dist1 := randgenerator.Float64()
		dist2 := randgenerator.Float64()
		tempDist := 0.0
		canditate1 := ""
		canditate2 := ""

		for j := 0; j < len(state.endpoints); j++ {
			tempDist = distributed[j]+tempDist
			if (dist1<=tempDist) {
				canditate1 = state.endpoints[j]
			}
		}
		tempDist = 0
		for j := 0; j < len(state.endpoints); j++ {
			tempDist = distributed[j]+tempDist
			if (dist2<=tempDist) {
				canditate2 = state.endpoints[j]
			}
		}
		bestcanditate := ""
		currentload1 := getload(canditate1)
		currentload2 := getload(canditate2)
		if (currentload1 >= 0 && currentload2 >= 0 && currentload1 < currentload2) {
			bestcanditate = canditate1
		} else if (currentload1 >= currentload2) {
			bestcanditate = canditate2
		}
		return bestcanditate, nil
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
			//state.tickets = make([]int, len(state.endpoints)) // Ziv's addition

			// Reset the round-robin index.
			state.index = 0
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
			//state.tickets = make([]int, len(state.endpoints)) // Ziv's addition

			// Reset the round-robin index.
			state.index = 0
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

