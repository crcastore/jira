from __future__ import annotations

from typing import Any


def build_bell_circuit() -> Any:
    from qiskit import QuantumCircuit

    circuit = QuantumCircuit(2, 2, name="bell_pair")
    circuit.h(0)
    circuit.cx(0, 1)
    circuit.measure([0, 1], [0, 1])
    return circuit


def build_bell_circuit_qasm() -> str:
    from qiskit import qasm2

    return qasm2.dumps(build_bell_circuit())