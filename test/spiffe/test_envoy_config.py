import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("envoy_config", ROOT / "scripts/generate-spiffe-envoy.py")
CONFIG = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CONFIG)
BOOTSTRAP_SPEC = importlib.util.spec_from_file_location("spire_bootstrap", ROOT / "docker/spiffe/bootstrap.py")
BOOTSTRAP = importlib.util.module_from_spec(BOOTSTRAP_SPEC)
BOOTSTRAP_SPEC.loader.exec_module(BOOTSTRAP)
QUIC_DOWNSTREAM = CONFIG.QUIC_TYPE + "QuicDownstreamTransport"
QUIC_UPSTREAM = CONFIG.QUIC_TYPE + "QuicUpstreamTransport"
H3_LISTENERS = {
    "service-b": {"inbound-service-b": 8443, "inbound-metrics": 8448},
    "payment": {"inbound-metrics": 8448},
    "service-a": {"inbound-metrics": 8448},
}


def tls_context(socket):
    """Return the TLS context of a TCP TLS or QUIC transport socket."""
    typed = socket["typed_config"]
    if typed["@type"] == QUIC_DOWNSTREAM:
        return typed["downstream_tls_context"]
    if typed["@type"] == QUIC_UPSTREAM:
        return typed["upstream_tls_context"]
    return typed


def listener_named(role, name):
    return next(item for item in CONFIG.build_config(role)["static_resources"]["listeners"] if item["name"] == name)


def cluster_named(role, name):
    return next(item for item in CONFIG.build_config(role)["static_resources"]["clusters"] if item["name"] == name)


class EnvoyIdentityTests(unittest.TestCase):
    def test_metrics_listener_requires_collector_and_only_metrics_path(self):
        for role in ("service-a", "service-b", "payment"):
            listeners = CONFIG.build_config(role)["static_resources"]["listeners"]
            listener = next(item for item in listeners if item["name"] == "inbound-metrics")
            chain = listener["filter_chains"][0]
            tls = tls_context(chain["transport_socket"])
            self.assertTrue(tls["require_client_certificate"])
            names = tls["common_tls_context"]["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
            self.assertEqual(names[0]["matcher"]["exact"], CONFIG.spiffe_id("metrics-collector"))
            routes = chain["filters"][0]["typed_config"]["route_config"]["virtual_hosts"][0]["routes"]
            self.assertEqual(len(routes), 1)
            self.assertEqual(routes[0]["match"]["path"], "/metrics")
            self.assertEqual(routes[0]["match"]["headers"][0]["string_match"]["exact"], "GET")

    def test_collector_only_reaches_metrics_ports(self):
        config = CONFIG.build_config("metrics-collector")
        self.assertEqual(len(config["static_resources"]["listeners"]), 3)
        secured = [item for item in config["static_resources"]["clusters"] if "transport_socket" in item]
        self.assertEqual(len(secured), 3)
        for cluster in secured:
            endpoint = cluster["load_assignment"]["endpoints"][0]["lb_endpoints"][0]["endpoint"]["address"]["socket_address"]
            self.assertEqual(endpoint["port_value"], 8448)
        for listener in config["static_resources"]["listeners"]:
            self.assertEqual(listener["address"]["socket_address"]["address"], "127.0.0.1")

    def test_server_requires_exact_workload_identity(self):
        listener = listener_named("service-b", "inbound-service-b")
        tls = tls_context(listener["filter_chains"][0]["transport_socket"])
        self.assertTrue(tls["require_client_certificate"])
        common = tls["common_tls_context"]
        self.assertEqual(common["tls_params"]["tls_maximum_protocol_version"], "TLSv1_3")
        self.assertEqual(common["tls_certificate_sds_secret_configs"][0]["name"], CONFIG.spiffe_id("service-b"))
        names = common["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
        self.assertEqual(names, [{"san_type": "URI", "matcher": {"exact": CONFIG.spiffe_id("service-a")}}])

    def test_database_accepts_exact_workload_identities(self):
        config = CONFIG.build_config("vtgate")
        for listener in config["static_resources"]["listeners"]:
            tls = tls_context(listener["filter_chains"][0]["transport_socket"])
            self.assertTrue(tls["require_client_certificate"])
            matcher = tls["common_tls_context"]["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
            peer = "payment" if listener["name"] == "inbound-payment-vitess-grpc" else "service-b"
            self.assertEqual(matcher[0]["matcher"]["exact"], CONFIG.spiffe_id(peer))

    def test_payment_accepts_only_service_b(self):
        listener = listener_named("payment", "inbound-payment")
        tls = tls_context(listener["filter_chains"][0]["transport_socket"])
        self.assertTrue(tls["require_client_certificate"])
        matcher = tls["common_tls_context"]["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
        self.assertEqual(matcher[0]["matcher"]["exact"], CONFIG.spiffe_id("service-b"))

    def test_outbound_listeners_are_loopback_only(self):
        for role in ("service-a", "service-b", "payment", "unauthorized", "metrics-collector"):
            for listener in CONFIG.build_config(role)["static_resources"]["listeners"]:
                if listener["name"].startswith("outbound-"):
                    self.assertEqual(listener["address"]["socket_address"]["address"], "127.0.0.1")

    def test_no_static_private_credentials(self):
        import json
        for role in ("service-a", "service-b", "payment", "vtgate", "unauthorized", "metrics-collector"):
            text = json.dumps(CONFIG.build_config(role))
            self.assertNotIn("private_key", text)
            self.assertNotIn("certificate_chain", text)
            self.assertNotIn("ACCEPT_UNTRUSTED", text)
            self.assertIn("tls_certificate_sds_secret_configs", text)

    def test_clients_verify_exact_server_identity(self):
        for role, peer in (("service-a", "service-b"), ("payment", "vtgate")):
            config = CONFIG.build_config(role)
            for cluster in config["static_resources"]["clusters"]:
                if "transport_socket" not in cluster:
                    continue
                tls = tls_context(cluster["transport_socket"])["common_tls_context"]
                names = tls["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
                self.assertEqual(names, [{"san_type": "URI", "matcher": {"exact": CONFIG.spiffe_id(peer)}}])

    def test_service_b_verifies_payment_and_vitess(self):
        config = CONFIG.build_config("service-b")
        for cluster in config["static_resources"]["clusters"]:
            if "transport_socket" not in cluster:
                continue
            expected = "payment" if cluster["name"] == "payment-mtls" else "vtgate"
            tls = tls_context(cluster["transport_socket"])["common_tls_context"]
            names = tls["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
            self.assertEqual(names[0]["matcher"]["exact"], CONFIG.spiffe_id(expected))


    def test_mesh_http_listeners_are_quic_only(self):
        for role, expected in H3_LISTENERS.items():
            listeners = CONFIG.build_config(role)["static_resources"]["listeners"]
            for name, port in expected.items():
                listener = next(item for item in listeners if item["name"] == name)
                address = listener["address"]["socket_address"]
                self.assertEqual((address["protocol"], address["port_value"]), ("UDP", port))
                self.assertIn("quic_options", listener["udp_listener_config"])
                chain = listener["filter_chains"][0]
                self.assertEqual(chain["transport_socket"]["typed_config"]["@type"], QUIC_DOWNSTREAM)
                manager = chain["filters"][0]["typed_config"]
                self.assertEqual(manager["codec_type"], "HTTP3")
                # Fail closed: no TCP twin offers an HTTP/2 or HTTP/1.1 fallback on the same port.
                twins = [item for item in listeners if item is not listener and item["address"]["socket_address"].get("port_value") == port]
                self.assertEqual(twins, [])

    def test_quic_server_disables_resumption_and_early_data(self):
        for role, expected in H3_LISTENERS.items():
            for name in expected:
                typed = listener_named(role, name)["filter_chains"][0]["transport_socket"]["typed_config"]
                self.assertIs(typed["enable_early_data"], False)
                self.assertIs(typed["enable_resumption"], False)
                self.assertTrue(typed["downstream_tls_context"]["require_client_certificate"])

    def test_mesh_http_clusters_speak_http3_with_dns_sni(self):
        expected = {
            "service-a": ["service-b-mtls"],
            "unauthorized": ["service-b-mtls"],
            "metrics-collector": None,
        }
        for role, names in expected.items():
            clusters = CONFIG.build_config(role)["static_resources"]["clusters"]
            selected = [item for item in clusters if names is None and "transport_socket" in item or names is not None and item["name"] in names]
            self.assertTrue(selected)
            for cluster in selected:
                socket = cluster["transport_socket"]["typed_config"]
                self.assertEqual(socket["@type"], QUIC_UPSTREAM)
                host = cluster["load_assignment"]["endpoints"][0]["lb_endpoints"][0]["endpoint"]["address"]["socket_address"]["address"]
                self.assertEqual(socket["upstream_tls_context"]["sni"], host)
                options = cluster["typed_extension_protocol_options"][CONFIG.HTTP_OPTIONS_KEY]["explicit_http_config"]
                self.assertEqual(list(options), ["http3_protocol_options"])

    def test_quic_server_identities_carry_matching_dns_names(self):
        # Envoy's QUIC client verifies the leaf against SNI, so every QUIC server SVID needs a DNS SAN equal to the SNI.
        servers = {"envoy-b": "service-b", "envoy-payment": "payment", "envoy-a": "service-a"}
        for host, role in servers.items():
            self.assertIn(host, BOOTSTRAP.DNS_NAMES[role])
        for role in ("vtgate", "unauthorized", "metrics-collector"):
            self.assertNotIn(role, BOOTSTRAP.DNS_NAMES)

    def test_grpc_hops_remain_tcp_tls_passthrough(self):
        # Envoy's HTTP/3 upstream codec turns gRPC trailers-only error responses into
        # HEADERS + empty DATA(END_STREAM), which gRPC clients reject; keep gRPC on L4 mTLS.
        inbound = [listener_named("payment", "inbound-payment")] + CONFIG.build_config("vtgate")["static_resources"]["listeners"]
        for listener in inbound:
            self.assertNotIn("udp_listener_config", listener)
            self.assertNotEqual(listener["address"]["socket_address"].get("protocol"), "UDP")
            chain = listener["filter_chains"][0]
            self.assertNotIn("Quic", chain["transport_socket"]["typed_config"]["@type"])
            self.assertEqual(chain["filters"][0]["name"], "envoy.filters.network.tcp_proxy")
        for role in ("service-b", "unauthorized"):
            payment = cluster_named(role, "payment-mtls")
            self.assertNotIn("Quic", payment["transport_socket"]["typed_config"]["@type"])
            self.assertNotIn("typed_extension_protocol_options", payment)


if __name__ == "__main__":
    unittest.main()
