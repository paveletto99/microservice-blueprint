#!/usr/bin/env python3
"""Generate public Envoy configuration. Private SVIDs are streamed by SPIRE SDS."""
import json
from pathlib import Path

TRUST_DOMAIN = "service-auth.test"
SDS = {"resource_api_version": "V3", "api_config_source": {"api_type": "GRPC", "transport_api_version": "V3", "grpc_services": [{"envoy_grpc": {"cluster_name": "spire_agent"}}]}}
TLS_TYPE = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3."
QUIC_TYPE = "type.googleapis.com/envoy.extensions.transport_sockets.quic.v3."
HTTP_OPTIONS_KEY = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"
HTTP_OPTIONS_TYPE = "type.googleapis.com/" + HTTP_OPTIONS_KEY
HTTP_TYPE = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"


def spiffe_id(role):
    return "spiffe://" + TRUST_DOMAIN + "/" + role


def common_tls(role, peer):
    return {
        "tls_params": {"tls_minimum_protocol_version": "TLSv1_3", "tls_maximum_protocol_version": "TLSv1_3"},
        "tls_certificate_sds_secret_configs": [{"name": spiffe_id(role), "sds_config": SDS}],
        "combined_validation_context": {
            "default_validation_context": {"match_typed_subject_alt_names": [{"san_type": "URI", "matcher": {"exact": spiffe_id(peer)}}]},
            "validation_context_sds_secret_config": {"name": "spiffe://" + TRUST_DOMAIN, "sds_config": SDS},
        },
    }


def tls_socket(role, peer, downstream=False):
    """TLS 1.3 over TCP for the opaque tcp_proxy hops (payment gRPC, Vitess gRPC/MySQL)."""
    context = {"@type": TLS_TYPE + ("DownstreamTlsContext" if downstream else "UpstreamTlsContext"), "common_tls_context": common_tls(role, peer)}
    if downstream:
        context["require_client_certificate"] = True
    return {"name": "envoy.transport_sockets.tls", "typed_config": context}


def quic_socket(role, peer, downstream=False, sni=None):
    """TLS 1.3 inside QUIC for HTTP/3 hops; same SPIFFE SDS material and exact URI SAN policy as TCP TLS."""
    context = {"common_tls_context": common_tls(role, peer)}
    if downstream:
        context["require_client_certificate"] = True
        # QUIC does not re-validate a client certificate when a session is resumed, and Envoy only defaults
        # resumption off for inline validation contexts (this stack delivers its bundle over SDS). Resumption
        # also enables 0-RTT early data, which can be replayed. Every connection performs a full SVID validation.
        config = {"@type": QUIC_TYPE + "QuicDownstreamTransport", "downstream_tls_context": context, "enable_early_data": False, "enable_resumption": False}
    else:
        # Envoy's QUIC verifier also requires a DNS SAN equal to the SNI on the server SVID (see DNS_NAMES in
        # docker/spiffe/bootstrap.py); the SPIFFE ID check above remains the authorization decision.
        context["sni"] = sni
        config = {"@type": QUIC_TYPE + "QuicUpstreamTransport", "upstream_tls_context": context}
    return {"name": "envoy.transport_sockets.quic", "typed_config": config}


def protocol_options(protocol, max_connection_duration=None):
    explicit = {"h2": {"http2_protocol_options": {}}, "h3": {"http3_protocol_options": {}}}[protocol]
    options = {"@type": HTTP_OPTIONS_TYPE, "explicit_http_config": explicit}
    if max_connection_duration:
        options["common_http_protocol_options"] = {"max_connection_duration": max_connection_duration}
    return {HTTP_OPTIONS_KEY: options}


def cluster(name, host, port, role=None, peer=None, protocol=None):
    """protocol: None = raw TCP (TLS when role is set) or loopback HTTP/1.1; "h3" = mTLS HTTP/3 over QUIC."""
    result = {"name": name, "type": "STRICT_DNS", "connect_timeout": "2s", "load_assignment": {"cluster_name": name, "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"socket_address": {"address": host, "port_value": port}}}}]}]}}
    if protocol == "h3":
        result["transport_socket"] = quic_socket(role, peer, sni=host)
        result["typed_extension_protocol_options"] = protocol_options("h3", "30s")
    elif role:
        result["transport_socket"] = tls_socket(role, peer)
    return result


def listener(name, address, port, filters, socket=None, quic=False):
    chain = {"filters": filters}
    if socket:
        chain["transport_socket"] = socket
    result = {"name": name, "address": {"socket_address": {"address": address, "port_value": port}}, "filter_chains": [chain]}
    if quic:
        # HTTP/3 is QUIC over UDP: there is deliberately no TCP/HTTP2 fallback listener on these ports.
        result["address"]["socket_address"]["protocol"] = "UDP"
        result["udp_listener_config"] = {"quic_options": {}}
    return result


def quic_listener(name, port, stat_prefix, routes, role, peer):
    """Mesh-facing HTTP/3 listener that requires the exact peer SVID."""
    return listener(name, "0.0.0.0", port, http_filter(stat_prefix, routes, quic=True), quic_socket(role, peer, downstream=True), quic=True)


def tcp_filter(name, target):
    return [{"name": "envoy.filters.network.tcp_proxy", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy", "stat_prefix": name, "cluster": target, "idle_timeout": "30s", "max_downstream_connection_duration": "30s"}}]


def http_filter(name, routes, quic=False):
    manager = {
        "@type": HTTP_TYPE, "stat_prefix": name, "codec_type": "HTTP3" if quic else "AUTO", "normalize_path": True,
        "forward_client_cert_details": "SANITIZE", "stream_idle_timeout": "10s", "request_timeout": "10s",
        "common_http_protocol_options": {"idle_timeout": "30s", "max_connection_duration": "30s"},
        "route_config": {"name": name, "request_headers_to_remove": ["x-spiffe-id", "x-forwarded-client-cert"], "virtual_hosts": [{"name": name, "domains": ["*"], "routes": routes}]},
        "http_filters": [{"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}],
    }
    if quic:
        manager["http3_protocol_options"] = {}
    return [{"name": "envoy.filters.network.http_connection_manager", "typed_config": manager}]


def forward(target):
    return {"match": {"prefix": "/"}, "route": {"cluster": target, "timeout": "5s"}}


def probe_route(response=False):
    match = {"path": "/__spiffe_probe", "headers": [{"name": ":method", "string_match": {"exact": "GET"}}]}
    if response:
        return {"match": match, "direct_response": {"status": 200, "body": {"inline_string": "SPIFFE mTLS OK\n"}}}
    return {"match": match, "route": {"cluster": "service-b-mtls", "timeout": "5s"}}


def metrics_route(target):
    return {"match": {"path": "/metrics", "headers": [{"name": ":method", "string_match": {"exact": "GET"}}]}, "route": {"cluster": target, "timeout": "3s"}}


def collector_upstreams(role, clusters, listeners):
    for peer, host, port in (("service-a", "envoy-a", 18010), ("service-b", "envoy-b", 18011), ("payment", "envoy-payment", 18012)):
        name = "metrics-" + peer
        clusters.append(cluster(name, host, 8448, role, peer, protocol="h3"))
        listeners.append(listener("outbound-" + name, "127.0.0.1", port, http_filter(name, [metrics_route(name)])))


def build_config(role):
    if role not in ("service-a", "service-b", "payment", "vtgate", "unauthorized", "metrics-collector"):
        raise ValueError("Unknown workload role")
    agent = {"name": "spire_agent", "type": "STATIC", "connect_timeout": "1s", "typed_extension_protocol_options": protocol_options("h2"), "load_assignment": {"cluster_name": "spire_agent", "endpoints": [{"lb_endpoints": [{"endpoint": {"address": {"pipe": {"path": "/run/spire/agent/agent.sock"}}}}]}]}}
    clusters, listeners = [agent], []
    if role in ("service-a", "unauthorized"):
        clusters += [cluster("service-b-mtls", "envoy-b", 8443, role, "service-b", protocol="h3"), cluster("app-a", "127.0.0.1", 8080)]
        listeners += [listener("outbound-service-b", "127.0.0.1", 18000, http_filter("outbound", [forward("service-b-mtls")])), listener("local-ingress", "0.0.0.0", 18080, http_filter("ingress", [probe_route(), forward("app-a")]))]
        # Test-only rejection probes use this workload's own SVID, never borrowed private keys.
        if role == "unauthorized":
            clusters.append(cluster("vitess-mysql-mtls", "envoy-vitess", 8445, role, "vtgate"))
            listeners.append(listener("outbound-vitess-denied", "127.0.0.1", 15993, tcp_filter("db-denied", "vitess-mysql-mtls")))
            clusters.append(cluster("payment-mtls", "envoy-payment", 8447, role, "payment"))
            listeners.append(listener("outbound-payment-denied", "127.0.0.1", 15051, tcp_filter("payment-denied", "payment-mtls")))
            collector_upstreams(role, clusters, listeners)
    elif role == "service-b":
        clusters += [cluster("app-b", "127.0.0.1", 8080), cluster("vitess-grpc-mtls", "envoy-vitess", 8444, role, "vtgate"), cluster("vitess-mysql-mtls", "envoy-vitess", 8445, role, "vtgate"), cluster("payment-mtls", "envoy-payment", 8447, role, "payment")]
        listeners += [quic_listener("inbound-service-b", 8443, "inbound", [probe_route(True), forward("app-b")], role, "service-a"), listener("outbound-vitess-grpc", "127.0.0.1", 15991, tcp_filter("db-grpc", "vitess-grpc-mtls")), listener("outbound-vitess-mysql", "127.0.0.1", 15993, tcp_filter("db-mysql", "vitess-mysql-mtls")), listener("outbound-payment-grpc", "127.0.0.1", 15051, tcp_filter("payment-grpc", "payment-mtls"))]
    elif role == "payment":
        clusters += [cluster("payment-app", "127.0.0.1", 50051), cluster("vitess-grpc-mtls", "envoy-vitess", 8446, role, "vtgate")]
        listeners += [listener("inbound-payment", "0.0.0.0", 8447, tcp_filter("payment-inbound", "payment-app"), tls_socket(role, "service-b", downstream=True)), listener("outbound-vitess-grpc", "127.0.0.1", 15991, tcp_filter("payment-db", "vitess-grpc-mtls"))]
    elif role == "metrics-collector":
        collector_upstreams(role, clusters, listeners)
    else:
        for protocol, port, upstream in (("grpc", 8444, 15991), ("mysql", 8445, 15993)):
            name = "vitess-" + protocol
            clusters.append(cluster(name, "vitess", upstream))
            listeners.append(listener("inbound-" + name, "0.0.0.0", port, tcp_filter(name, name), tls_socket(role, "service-b", downstream=True)))
        listeners.append(listener("inbound-payment-vitess-grpc", "0.0.0.0", 8446, tcp_filter("payment-vitess-grpc", "vitess-grpc"), tls_socket(role, "payment", downstream=True)))
    if role in ("service-a", "service-b", "payment"):
        name = "application-metrics"
        clusters.append(cluster(name, "127.0.0.1", 9090 if role == "payment" else 8080))
        listeners.append(quic_listener("inbound-metrics", 8448, "metrics-inbound", [metrics_route(name)], role, "metrics-collector"))
    return {"node": {"id": "envoy-" + role, "cluster": "service-auth"}, "admin": {"address": {"socket_address": {"address": "127.0.0.1", "port_value": 9901}}}, "overload_manager": {"resource_monitors": [{"name": "envoy.resource_monitors.global_downstream_max_connections", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig", "max_active_downstream_connections": 256}}]}, "static_resources": {"listeners": listeners, "clusters": clusters}}


if __name__ == "__main__":
    output = Path(__file__).resolve().parent.parent / "docker/spiffe/envoy"
    output.mkdir(parents=True, exist_ok=True)
    for role in ("service-a", "service-b", "payment", "vtgate", "unauthorized", "metrics-collector"):
        (output / (role + ".json")).write_text(json.dumps(build_config(role), indent=2, sort_keys=True) + "\n")
