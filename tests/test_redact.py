from __future__ import annotations

import unittest

from qiskit_zap_ibm_demo.redact import REDACTED, redact_json, redact_text


class RedactionTests(unittest.TestCase):
    def test_redacts_bearer_tokens_in_text(self) -> None:
        text = redact_text("Authorization: Bearer fake-ibm-quantum-token")

        self.assertIn(REDACTED, text)
        self.assertNotIn("fake-ibm-quantum-token", text)

    def test_redacts_fake_tokens_outside_bearer_headers(self) -> None:
        text = redact_text("invalid token fake-ibm-quantum-token")

        self.assertIn(REDACTED, text)
        self.assertNotIn("fake-ibm-quantum-token", text)

    def test_redacts_sensitive_json_keys(self) -> None:
        value = redact_json(
            {
                "Authorization": "Bearer fake-token",
                "nested": {"api_token": "fake-token"},
                "safe": "keep-me",
            }
        )

        self.assertEqual(value["Authorization"], REDACTED)
        self.assertEqual(value["nested"]["api_token"], REDACTED)
        self.assertEqual(value["safe"], "keep-me")


if __name__ == "__main__":
    unittest.main()
