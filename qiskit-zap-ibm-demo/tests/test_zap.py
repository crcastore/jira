from __future__ import annotations

import unittest

from qiskit_zap_ibm_demo.zap import filter_messages_for_targets


class ZapTests(unittest.TestCase):
    def test_filter_messages_for_targets_matches_any_host(self) -> None:
        messages = [
            {"url": "https://quantum.cloud.ibm.com/runtime"},
            {"url": "http://example.local/runtime/jobs"},
        ]

        selected = filter_messages_for_targets(messages, ["ibm.com"])

        self.assertEqual(selected, [messages[0]])


if __name__ == "__main__":
    unittest.main()