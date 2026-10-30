import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("envoy_config", ROOT / "scripts/generate-spiffe-envoy.py")
CONFIG = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CONFIG)


class EnvoyIdentityTests(unittest.TestCase):
    def test_server_requires_exact_workload_identity(self):
        config = CONFIG.build_config("service-b")
        listener = config["static_resources"]["listeners"][0]
        tls = listener["filter_chains"][0]["transport_socket"]["typed_config"]
        self.assertTrue(tls["require_client_certificate"])
        common = tls["common_tls_context"]
        self.assertEqual(common["tls_params"]["tls_maximum_protocol_version"], "TLSv1_3")
        self.assertEqual(common["tls_certificate_sds_secret_configs"][0]["name"], CONFIG.spiffe_id("service-b"))
        names = common["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
        self.assertEqual(names, [{"san_type": "URI", "matcher": {"exact": CONFIG.spiffe_id("service-a")}}])

    def test_database_accepts_only_service_b(self):
        config = CONFIG.build_config("vtgate")
        for listener in config["static_resources"]["listeners"]:
            tls = listener["filter_chains"][0]["transport_socket"]["typed_config"]
            self.assertTrue(tls["require_client_certificate"])
            matcher = tls["common_tls_context"]["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
            self.assertEqual(matcher[0]["matcher"]["exact"], CONFIG.spiffe_id("service-b"))

    def test_outbound_listeners_are_loopback_only(self):
        for role in ("service-a", "service-b", "unauthorized"):
            for listener in CONFIG.build_config(role)["static_resources"]["listeners"]:
                if listener["name"].startswith("outbound-"):
                    self.assertEqual(listener["address"]["socket_address"]["address"], "127.0.0.1")

    def test_no_static_private_credentials(self):
        import json
        for role in ("service-a", "service-b", "vtgate", "unauthorized"):
            text = json.dumps(CONFIG.build_config(role))
            self.assertNotIn("private_key", text)
            self.assertNotIn("certificate_chain", text)
            self.assertNotIn("ACCEPT_UNTRUSTED", text)
            self.assertIn("tls_certificate_sds_secret_configs", text)

    def test_clients_verify_exact_server_identity(self):
        for role, peer in (("service-a", "service-b"), ("service-b", "vtgate")):
            config = CONFIG.build_config(role)
            for cluster in config["static_resources"]["clusters"]:
                if "transport_socket" not in cluster:
                    continue
                tls = cluster["transport_socket"]["typed_config"]["common_tls_context"]
                names = tls["combined_validation_context"]["default_validation_context"]["match_typed_subject_alt_names"]
                self.assertEqual(names, [{"san_type": "URI", "matcher": {"exact": CONFIG.spiffe_id(peer)}}])


if __name__ == "__main__":
    unittest.main()
