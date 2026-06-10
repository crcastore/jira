from __future__ import annotations

import unittest

from qiskit_zap_ibm_demo.circuit import build_bell_circuit_qasm


class CircuitTests(unittest.TestCase):
    def test_build_bell_circuit_qasm_contains_bell_circuit(self) -> None:
        qasm = build_bell_circuit_qasm()

        self.assertIn("OPENQASM", qasm.upper())
        self.assertIn("h q[0]", qasm)
        self.assertIn("cx q[0],q[1]", qasm)


if __name__ == "__main__":
    unittest.main()