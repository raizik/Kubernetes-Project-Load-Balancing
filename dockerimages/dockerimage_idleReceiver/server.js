const http = require('http')
var postData
const server = http.createServer(function(request, response) {
  //console.dir(request.param)

  //if (request.method == 'POST') {
    //console.log('POST')
    var body = ''
    request.on('data', function(data) {
      body += data
      //console.log('Partial body: ' + body)
           
        
    })
    request.on('end', function() {
      console.log('Body: ' + body)
      response.writeHead(200, {'Content-Type': 'application/json'})
      response.end('post received')
    })
   /* request.on('error', (e) => {
    console.error(e);
})*/

 // }
})

const port = 8081
//const host = '172.17.0.3'
server.listen(port)
console.log(`Listening at http://:${port}`)

