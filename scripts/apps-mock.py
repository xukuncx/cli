#!/usr/bin/env python3
"""Apps domain mock OAPI server.

Usage:
  python3 scripts/apps-mock.py [success|build_failed|app_not_found|http_503]

Verbose mode prints multipart boundary + first field name + auth header for
the publish endpoint (always-on; pass MOCK_VERBOSE=1 for all endpoints).
"""
import http.server, json, os, re, sys
from urllib.parse import urlparse, parse_qs

MODE = sys.argv[1] if len(sys.argv) > 1 else 'success'
VERBOSE = os.environ.get('MOCK_VERBOSE') == '1'
BASE = '/open-apis/spark/v1'  # 对齐 BOE 后端实际注册路径

def route(method, path):
    if method == 'POST' and path == f'{BASE}/apps':
        return 'create'
    if method == 'GET' and path == f'{BASE}/apps':
        return 'list'
    if method == 'PATCH' and re.match(rf'^{re.escape(BASE)}/apps/[^/]+$', path):
        return 'update'
    if method == 'PUT' and re.match(rf'^{re.escape(BASE)}/apps/[^/]+/access-scope$', path):
        return 'access_scope'
    if method == 'GET' and re.match(rf'^{re.escape(BASE)}/apps/[^/]+/access-scope$', path):
        return 'access_scope_get'
    if method == 'POST' and re.match(rf'^{re.escape(BASE)}/apps/[^/]+/upload_and_release_html_code$', path):
        return 'publish'
    return None

def build_response(action):
    if MODE == 'http_503':
        return None, 503, b'service unavailable'
    if action == 'publish' and MODE == 'build_failed':
        return {'code': 90001, 'msg': 'build failed: dependency conflict'}, 200, None
    if action == 'publish' and MODE == 'app_not_found':
        return {'code': 90002, 'msg': 'app not found or no permission'}, 200, None
    payloads = {
        'create':       {'app_id': 'app_mock_001', 'name': 'Mock App', 'icon_url': 'https://example.com/i.svg', 'created_at': '2026-05-18T10:00:00Z'},
        'update':       {'app_id': 'app_mock_001', 'name': 'Mock v2', 'updated_at': '2026-05-18T10:05:00Z'},
        'list':         {'items': [{'app_id': 'app_mock_001', 'name': 'Mock App'}], 'page_token': '', 'has_more': False},
        'access_scope': {},
        'access_scope_get': {
            'scope': 3,
            'users': ['ou_mock_user_1', 'ou_mock_user_2'],
            'departments': ['od_mock_dept_1'],
            'chats': ['oc_mock_chat_1'],
            'apply_config': {'enabled': True, 'approvers': ['ou_mock_approver']},
        },
        'publish':      {'url': 'http://localhost:8181/preview/app_mock_001'},
    }
    return {'code': 0, 'msg': 'success', 'data': payloads.get(action, {})}, 200, None

class H(http.server.BaseHTTPRequestHandler):
    def _handle(self, method):
        parsed = urlparse(self.path)
        n = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(n) if n else b''
        action = route(method, parsed.path)
        print(f'[mock {MODE}] {method} {parsed.path}  query={dict(parse_qs(parsed.query))}  body_bytes={len(body)}  -> action={action}', file=sys.stderr)
        if action == 'publish' or VERBOSE:
            auth = self.headers.get('Authorization', '<missing>')
            print(f'[mock]   Authorization: {auth[:40]}...', file=sys.stderr)
            ct = self.headers.get('Content-Type', '')
            if 'multipart' in ct:
                boundary = ct.split('boundary=')[-1]
                m = re.search(rb'name="([^"]+)"', body)
                print(f'[mock]   multipart boundary={boundary[:20]}...', file=sys.stderr)
                print(f'[mock]   first field name = {m.group(1).decode() if m else "<none>"}', file=sys.stderr)
        if action is None:
            self.send_response(404); self.end_headers(); return
        payload, code, raw = build_response(action)
        self.send_response(code)
        if raw is not None:
            self.send_header('Content-Type', 'text/plain'); self.end_headers(); self.wfile.write(raw); return
        self.send_header('Content-Type', 'application/json'); self.end_headers()
        self.wfile.write(json.dumps(payload).encode())

    def do_GET(self):    self._handle('GET')
    def do_POST(self):   self._handle('POST')
    def do_PATCH(self):  self._handle('PATCH')
    def do_PUT(self):    self._handle('PUT')
    def log_message(self, *a): pass

print(f'[mock] mode={MODE} verbose={VERBOSE}  listening on http://127.0.0.1:8181', file=sys.stderr)
http.server.HTTPServer(('127.0.0.1', 8181), H).serve_forever()
