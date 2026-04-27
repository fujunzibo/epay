from http.server import BaseHTTPRequestHandler, HTTPServer

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        l = int(self.headers.get("Content-Length", 0))
        b = self.rfile.read(l).decode()
        print("TOPUP REQ:", b)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

HTTPServer(("127.0.0.1", 19090), H).serve_forever()