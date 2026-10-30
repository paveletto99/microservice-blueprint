#!/usr/bin/env python3
"""Generate public Envoy configuration. Private SVIDs are streamed by SPIRE SDS."""
import json
from pathlib import Path

TRUST_DOMAIN = "service-auth.test"
SDS = {"resource_api_version": "V3", "api_config_source": {"api_type": "GRPC", "transport_api_version": "V3", "grpc_services": [{"envoy_grpc": {"cluster_name": "spire_agent"}}]}}
TLS_TYPE = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3."
HTTP_TYPE = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"


def spiffe_id(role):
    return "spiffe://" + TRUST_DOMAIN + "/" + role


def tls_socket(role, peer, downstream=False, http=False):
    common = {
        "tls_params": {"tls_minimum_protocol_version": "TLSv1_3", "tls_maximum_protocol_version": "TLSv1_3"},
        "tls_certificate_sds_secret_configs": [{"name": spiffe_id(role), "sds_config": SDS}],
        "combined_validation_context": {
            "default_validation_context": {"match_typed_subject_alt_names": [{"san_type": "URI", "matcher": {"exact": spiffe_id(peer)}}]},
            "validation_context_sds_secret_config": {"name": "spiffe://" + TRUST_DOMAIN, "sds_config": SDS},
        },
    }
    if http:
        common["alpn_protocols"] = ["h2"]
    context = {"@type": TLS_TYPE + ("DownstreamTlsContext" if downstream else "UpstreamTlsContext"), "common_tls_context": common}
    if downstream:
        context["require_client_certificate"] = True
    return {"name": "envoy.transport_sockets.tls", "typed_config": context}


def cluster(name, host, port, role=None, peer=None, http=False):
    result = {"name": name, "type": "STRICT_DNS", "connect_timeout": "2s", "load_assignment": {"cluster_name": name, "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"socket_address": {"address": host, "port_value": port}}}}]}]}}
    if role:
        result["transport_socket"] = tls_socket(role, peer, http=http)
    if http:
        result["typed_extension_protocol_options"] = {"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {"@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions", "explicit_http_config": {"http2_protocol_options": {}}, "common_http_protocol_options": {"max_connection_duration": "30s"}}}
    return result


def listener(name, address, port, filters, socket=None):
    chain = {"filters": filters}
    if socket:
        chain["transport_socket"] = socket
    return {"name": name, "address": {"socket_address": {"address": address, "port_value": port}}, "filter_chains": [chain]}


def tcp_filter(name, target):
    return [{"name": "envoy.filters.network.tcp_proxy", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy", "stat_prefix": name, "cluster": target, "idle_timeout": "30s", "max_downstream_connection_duration": "30s"}}]


def http_filter(name, routes):
    return [{"name": "envoy.filters.network.http_connection_manager", "typed_config": {
        "@type": HTTP_TYPE, "stat_prefix": name, "codec_type": "AUTO", "normalize_path": True,
        "forward_client_cert_details": "SANITIZE", "stream_idle_timeout": "10s", "request_timeout": "10s",
        "common_http_protocol_options": {"idle_timeout": "30s", "max_connection_duration": "30s"},
        "route_config": {"name": name, "request_headers_to_remove": ["x-spiffe-id", "x-forwarded-client-cert"], "virtual_hosts": [{"name": name, "domains": ["*"], "routes": routes}]},
        "http_filters": [{"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}],
    }}]


def forward(target):
    return {"match": {"prefix": "/"}, "route": {"cluster": target, "timeout": "5s"}}


def probe_route(response=False):
    match = {"path": "/__spiffe_probe", "headers": [{"name": ":method", "string_match": {"exact": "GET"}}]}
    if response:
        return {"match": match, "direct_response": {"status": 200, "body": {"inline_string": "SPIFFE mTLS OK\n"}}}
    return {"match": match, "route": {"cluster": "service-b-mtls", "timeout": "5s"}}


def build_config(role):
    if role not in ("service-a", "service-b", "vtgate", "unauthorized"):
        raise ValueError("Unknown workload role")
    agent = {"name": "spire_agent", "type": "STATIC", "connect_timeout": "1s", "typed_extension_protocol_options": {"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {"@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions", "explicit_http_config": {"http2_protocol_options": {}}}}, "load_assignment": {"cluster_name": "spire_agent", "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"pipe": {"path": "/run/spire/agent/agent.sock"}}}}]}]}}
    clusters, listeners = [agent], []
    if role in ("service-a", "unauthorized"):
        clusters += [cluster("service-b-mtls", "envoy-b", 8443, role, "service-b", http=True), cluster("app-a", "127.0.0.1", 8080)]
        listeners += [listener("outbound-service-b", "127.0.0.1", 18000, http_filter("outbound", [forward("service-b-mtls")])), listener("local-ingress", "0.0.0.0", 18080, http_filter("ingress", [probe_route(), forward("app-a")]))]
        # Test-only rejection probes use this workload's own SVID, never borrowed private keys.
        if role == "unauthorized":
            clusters.append(cluster("vitess-mysql-mtls", "envoy-vitess", 8445, role, "vtgate"))
            listeners.append(listener("outbound-vitess-denied", "127.0.0.1", 15993, tcp_filter("db-denied", "vitess-mysql-mtls")))
    elif role == "service-b":
        clusters += [cluster("app-b", "127.0.0.1", 8080), cluster("vitess-grpc-mtls", "envoy-vitess", 8444, role, "vtgate"), cluster("vitess-mysql-mtls", "envoy-vitess", 8445, role, "vtgate")]
        listeners += [listener("inbound-service-b", "0.0.0.0", 8443, http_filter("inbound", [probe_route(True), forward("app-b")]), tls_socket(role, "service-a", downstream=True, http=True)), listener("outbound-vitess-grpc", "127.0.0.1", 15991, tcp_filter("db-grpc", "vitess-grpc-mtls")), listener("outbound-vitess-mysql", "127.0.0.1", 15993, tcp_filter("db-mysql", "vitess-mysql-mtls"))]
    else:
        for protocol, port, upstream in (("grpc", 8444, 15991), ("mysql", 8445, 15993)):
            name = "vitess-" + protocol
            clusters.append(cluster(name, "vitess", upstream))
            listeners.append(listener("inbound-" + name, "0.0.0.0", port, tcp_filter(name, name), tls_socket(role, "service-b", downstream=True)))
    return {"node": {"id": "envoy-" + role, "cluster": "service-auth"}, "admin": {"address": {"socket_address": {"address": "127.0.0.1", "port_value": 9901}}}, "overload_manager": {"resource_monitors": [{"name": "envoy.resource_monitors.global_downstream_max_connections", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig", "max_active_downstream_connections": 256}}]}, "static_resources": {"listeners": listeners, "clusters": clusters}}


if __name__ == "__main__":
    output = Path(__file__).resolve().parent.parent / "docker/spiffe/envoy"
    output.mkdir(parents=True, exist_ok=True)
    for role in ("service-a", "service-b", "vtgate", "unauthorized"):
        (output / (role + ".json")).write_text(json.dumps(build_config(role), indent=2, sort_keys=True) + "\n")
