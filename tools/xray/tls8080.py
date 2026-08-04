import http.server, ssl
httpd = http.server.HTTPServer(('127.0.0.1', 8081), http.server.SimpleHTTPRequestHandler)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain('/tmp/fb_cert.pem', '/tmp/fb_key.pem')
httpd.socket = ctx.wrap_socket(httpd.socket, server_side=True)
print('tls server on 8080', flush=True)
httpd.serve_forever()