from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from qiskit import QuantumCircuit


def build_bell_circuit() -> QuantumCircuit:
    from qiskit import QuantumCircuit

    circuit = QuantumCircuit(2, 2, name="bell_pair")
    circuit.h(0)
    circuit.cx(0, 1)
    circuit.measure([0, 1], [0, 1])
    return circuit


def circuit_to_qasm(circuit: QuantumCircuit) -> str:
    from qiskit import qasm2

    return qasm2.dumps(circuit)


def build_bell_circuit_qasm() -> str:
    return circuit_to_qasm(build_bell_circuit())