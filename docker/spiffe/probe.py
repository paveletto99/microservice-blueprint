"""Transport-only fixture/probes. No OAuth tokens, SVID keys, or certificate files are read."""
import http.server
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.request


def get(url, headers=None):
    request = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(request, timeout=5) as response:
        return response.status, response.read(65536)


def denied_http(url, headers=None):
    try:
        status, _ = get(url, headers)
    except urllib.error.HTTPError as error:
        if error.code != 503:
            raise RuntimeError("Unexpected proxy denial status") from None
        return
    except (urllib.error.URLError, ConnectionError, TimeoutError):
        return
    raise RuntimeError("Unauthorized or plaintext request reached HTTP: " + str(status))


def denied_tcp(host, port):
    try:
        connection = socket.create_connection((host, port), timeout=3)
    except OSError:
        return
    connection.close()
    raise RuntimeError("Plaintext backend was reachable from a mesh peer")


def certificate_serials():
    _, body = get("http://127.0.0.1:9901/certs")
    data = json.loads(body)
    return sorted(str(cert["serial_number"]) for item in data.get("certificates", []) for cert in item.get("cert_chain", []) if "serial_number" in cert)


def main():
    command = sys.argv[1]
    if command == "backend":
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path == "/metrics":
                    self.send_response(200)
                    self.send_header("Content-Type", "text/plain; version=0.0.4")
                    self.end_headers()
                    self.wfile.write(b"spiffe_test_backend_up 1\n")
                    return
                if self.path != "/__test_backend":
                    self.send_error(404)
                    return
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"loopback-only-test-backend\n")
            def log_message(self, *_):
                pass
        http.server.HTTPServer(("127.0.0.1", 8080), Handler).serve_forever()
    elif command == "collection":
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            _, body = get("http://127.0.0.1:9090/api/v1/query?query=up")
            result = json.loads(body)
            jobs = {item["metric"]["job"]: item["value"][1] for item in result.get("data", {}).get("result", [])}
            if all(jobs.get(job) == "1" for job in ("service-a", "service-b", "payment")):
                break
            time.sleep(1)
        else:
            raise RuntimeError("Prometheus did not scrape all authenticated targets")
        for port in (18010, 18011, 18012):
            status, body = get("http://127.0.0.1:" + str(port) + "/metrics")
            if status != 200 or not body:
                raise RuntimeError("Authenticated metrics route failed")
            try:
                get("http://127.0.0.1:" + str(port) + "/orders")
            except urllib.error.HTTPError as error:
                if error.code != 404:
                    raise RuntimeError("Unexpected collector business-route rejection") from None
            else:
                raise RuntimeError("Collector could reach a business route")
        _, body = get("http://127.0.0.1:9090/api/v1/query?query=service_trace_export_enabled%7Bjob%3D%22payment%22%7D")
        if not json.loads(body).get("data", {}).get("result"):
            raise RuntimeError("Real payment telemetry was not ingested")
        print("Prometheus ingested all three metrics targets over SPIFFE mTLS; business routes remain inaccessible.")
    elif command == "collection-denied":
        for port in (18010, 18011, 18012):
            denied_http("http://127.0.0.1:" + str(port) + "/metrics")
            denied_http("http://127.0.0.1:" + str(port) + "/metrics", {"x-spiffe-id": "spiffe://service-auth.test/metrics-collector", "x-forwarded-client-cert": "URI=spiffe://service-auth.test/metrics-collector"})
        denied_tcp("envoy-payment", 9090)
        denied_tcp("envoy-collector", 9090)
        print("Unauthorized identity, forged collector headers and raw metrics bypass were rejected.")
    elif command == "allowed":
        status, body = get("http://127.0.0.1:18000/__test_backend")
        if status != 200 or body != b"loopback-only-test-backend\n":
            raise RuntimeError("Authenticated proxy forwarding failed")
        print("Authenticated mTLS forwarding to the loopback backend passed.")
    elif command == "denied":
        denied_http("http://127.0.0.1:18080/__spiffe_probe")
        denied_http("http://127.0.0.1:18080/__spiffe_probe", {"x-spiffe-id": "spiffe://service-auth.test/service-a", "x-forwarded-client-cert": "URI=spiffe://service-auth.test/service-a"})
        denied_http("http://envoy-b:8443/__spiffe_probe")
        denied_tcp("envoy-b", 8080)
        denied_tcp(os.environ["VITESS_BACKEND_IP"], 15991)
        denied_tcp(os.environ["VITESS_BACKEND_IP"], 15993)
        denied_tcp(os.environ["VITESS_BACKEND_IP"], 15992)
        denied_tcp(os.environ["VITESS_BACKEND_IP"], 15990)
        print("Wrong SPIFFE identity, forged headers, plaintext TLS and backend bypasses rejected.")
    elif command == "renewal":
        original = certificate_serials()
        if not original:
            raise RuntimeError("No workload certificate loaded")
        deadline = time.monotonic() + 100
        while time.monotonic() < deadline:
            time.sleep(3)
            status, _ = get("http://127.0.0.1:18000/__spiffe_probe")
            if status != 200:
                raise RuntimeError("Traffic failed during SVID renewal")
            if certificate_serials() != original:
                print("SPIRE SVID renewed through SDS; authenticated traffic still succeeds.")
                return
        raise RuntimeError("SVID did not renew within the bounded test interval")
    else:
        raise RuntimeError("Unknown transport probe")


if __name__ == "__main__":
    try:
        main()
    except Exception:
        sys.exit("SPIFFE transport probe failed (no credentials or certificate material printed).")
