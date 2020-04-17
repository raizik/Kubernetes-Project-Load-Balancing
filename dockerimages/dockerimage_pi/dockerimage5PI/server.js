const http = require('http');
const fs = require('fs');
const url = require('url');

var podname = process.env.HOSTNAME;
var pod_ip
//find ip
'use strict';

var os = require('os');
var ifaces = os.networkInterfaces();

Object.keys(ifaces).forEach(function (ifname) {
  //var alias = 0;

  ifaces[ifname].forEach(function (iface) {
    if ('IPv4' !== iface.family || iface.internal !== false) {
      // skip over internal (i.e. 127.0.0.1) and non-ipv4 addresses
      return;
    }
     pod_ip = iface.address;
     return;
    //if (alias >= 1) {
      // this single interface has multiple ipv4 addresses
      //console.log(ifname + ':' + alias, iface.address);
    //} else {
      // this interface has only one ipv4 adress
     // console.log(ifname, iface.address);
    //}
    //++alias;
  });
});
var pod_port = "8080";
if (podname === null) podname = "unknown";

//heterogenity
var mu = 10;
//var rate = (podname.includes("0")) ? (mu) : (5 * mu);
var podid = podname.match(/\d+/)[0];
if (podid >= 0 && podid <= 31)
  rate = mu;
else if (podid >= 32)
  rate = 5 * mu;


//HTTP POST
var postData = JSON.stringify({msg: 'IDLE, ' + pod_ip + ':' + pod_port})

var options = {
  hostname: '10.244.0.4',  //control plane node
  port: 8081,              //NodePort
  path: '/postjson',
  method: 'POST',
  headers: {
       'Content-Type': 'application/json',
       'Content-Length': postData.length
     }
};





function handleRequest(request, response) {
  console.log('Received request for URL: ' + request.url);
  logConnection(request.url + "\n");
  var url_parts = url.parse(request.url, true);
  var query = url_parts.query;


  if (request.url.startsWith("/delay"))
  {
	  delay(request, response);
  }
  else if (request.url.startsWith("/count"))
  {
	response.writeHead(200);
	response.end(connections.size.toString());
  }
  else
  {
	response.writeHead(200);
	response.write(podname);
	response.write("\r\n");
	//printing current date and time for creating dynamic pages in accordance with StatefulSet deployments
	var currentdate = new Date();
    var datetime = "Last Sync: " + currentdate.getDate() + "/"
                + (currentdate.getMonth()+1)  + "/"
                + currentdate.getFullYear() + " @ "
                + currentdate.getHours() + ":"
                + currentdate.getMinutes() + ":"
                + currentdate.getSeconds();
	response.write(datetime);
	response.write("\r\n");
	response.end('Hello world!\r\n');
  }
      //HTTP POST
      var req = http.request(options, (res) => {
  console.log('statusCode:', res.statusCode);
  console.log('headers:', res.headers);

  res.on('data', (d) => {
    process.stdout.write(d);
  });
});

req.on('error', (e) => {
  console.error(e);
});
    //post IDLE

    req.end(postData);
    logPost("idle" + "\n");

    //req.end();
}

const connections = new Set();
function expovariate(lambda)
{
	return (-Math.log(1.0 - Math.random())/lambda) * 1000.0
}

//auxilary method for getting a random integer between min and max
function getRandomBtw(min, max) {
    return Math.random() * (max - min) + min;
}

//geometric distribution function
function geometricdist(lambda, maxTrials)
{
    var success_slot = getRandomBtw(1, maxTrials);
    return (Math.pow(1 - (1/lambda), success_slot - 1) * (1/lambda))
}

function getQueueAndServiceTime()
{
	// Find latest reply Time
	var requestTime = Date.now()
	var latest = requestTime;
	for (var r of connections)
		if (r._sim_replyTime > latest)
			latest = r._sim_replyTime;

	// Service time distributes geometrically with parameter (1/rate)
	//maxTrials is the maximum number of slots until 1st successful transmission
	//TODO: how should this be set?
    //var maxTrials =
    //var geometricRes = geometricdist(rate, maxTrials)
    var expovariateResult = expovariate(rate)
	return {
			"replyTime": latest + expovariateResult,
			"timeWaiting": (latest + expovariateResult) - requestTime,
			"timeInService": expovariateResult
			};

}

function delay(request, response)
{
	var res = getQueueAndServiceTime();
	response._sim_replyTime = res.replyTime

	connections.add(response);
	response.writeHead(200);
	response.write(podname + "\r\n");
	response.write(res.timeWaiting.toString() + "\r\n");
	response.write(res.timeInService.toString() + "\r\n");
	response.end(connections.size.toString() + "\r\n");

	setTimeout(() => deleteFromConnectionsList(response), res.timeWaiting);
}


function deleteFromConnectionsList(response)
{
  connections.delete(response);
}


function logConnection(text)
{
 fs.appendFile("connections.log", text, (err) => {});

}
//racheli

function logPost(text)
{
 fs.appendFile("posthttptest.log", text, (err) => {});

}
var www = http.createServer(handleRequest);
www.listen(8080);
