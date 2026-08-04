import http.server, os

SUB = '''proxies:
  - name: "SUB-XHTTP"
    type: vless
    server: 127.0.0.1
    port: 8444
    uuid: 34903645-2a21-4d15-b8e4-57784431114d
    network: xhttp
    udp: true
    tls: true
    servername: www.apple.com
    client-fingerprint: chrome
    reality-opts:
      public-key: SUjqUBVYjar8Q1-XQnFy4u86-ISFWyH3RKm3BoNPcDQ
      short-id: abcdef0123456789
    xhttp-opts:
      mode: stream-up
      path: /
      session-id-placement: path
      seq-placement: path
'''

ETAG = '"v2-xyz789"'
USERINFO = 'upload=1000000; download=2000000; total=10000000000; expire=1900000000'

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        try:
            with open('/tmp/sub_server.log', 'a') as f:
                f.write('UA=%s IfNoneMatch=%s %s\n' % (self.headers.get('User-Agent', ''), self.headers.get('If-None-Match', ''), fmt % args))
        except Exception:
            pass
    def do_GET(self):
        if self.path != '/sub':
            self.send_response(404); self.end_headers(); return
        if self.headers.get('If-None-Match') == ETAG:
            self.send_response(304)
            self.send_header('ETag', ETAG)
            self.end_headers()
            return
        body = SUB.encode()
        self.send_response(200)
        self.send_header('Content-Type', 'text/yaml')
        self.send_header('Subscription-Userinfo', USERINFO)
        self.send_header('ETag', ETAG)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

http.server.HTTPServer(('127.0.0.1', 8090), H).serve_forever()