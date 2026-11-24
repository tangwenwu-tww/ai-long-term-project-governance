"""Deterministic black-box verification for maturity-driven governance promotion."""

from __future__ import annotations

import hashlib
import json
import tempfile
from pathlib import Path

import gate


def main() -> int:
    project = Path(__file__).resolve().parents[1]
    assessment = gate.assess_phase(gate.maturity(project))
    assert assessment["recommended_phase"] == 3
    gate_sizes = [len(gate.gates_for_phase(phase)) for phase in range(5)]
    assert gate_sizes == sorted(gate_sizes)

    with tempfile.TemporaryDirectory() as temp:
        target = Path(temp)
        governance_dir = target / ".ai-governance"
        governance_dir.mkdir()
        config = gate.load_json(project / ".ai-governance" / "governance.json")
        config["phase"] = 1
        (governance_dir / "governance.json").write_text(json.dumps(config), encoding="utf-8")
        (governance_dir / "maturity.json").write_text(json.dumps({
            "core_hypothesis_proven": True,
            "first_external_user": True,
            "team_size": 2,
        }), encoding="utf-8")
        trusted_token = "isolated-human-token"
        (governance_dir / "trust.json").write_text(json.dumps({
            "promotion_token_sha256": {
                "1->2": hashlib.sha256(trusted_token.encode("utf-8")).hexdigest()
            }
        }), encoding="utf-8")
        approval_path = target / "approval.json"

        class Args:
            project = str(target)
            approval_file = str(approval_path)

        forged = {"from_phase": 1, "to_phase": 2, "approved_by": "agent", "token": "forged"}
        approval_path.write_text(json.dumps(forged), encoding="utf-8")
        try:
            gate.promote_command(Args())
        except gate.GateError:
            forged_rejected = True
        else:
            raise AssertionError("forged approval unexpectedly succeeded")

        valid = {"from_phase": 1, "to_phase": 2, "approved_by": "human-owner", "token": trusted_token}
        approval_path.write_text(json.dumps(valid), encoding="utf-8")
        assert gate.promote_command(Args()) == 0
        promoted = gate.governance(target)
        assert promoted["phase"] == 2
        assert promoted["phase_history"][-1]["approved_by"] == "human-owner"
        assert len(promoted["phase_history"][-1]["approval_sha256"]) == 64

    print(json.dumps({
        "current_recommended": assessment["recommended_phase"],
        "forged_rejected": forged_rejected,
        "gate_sizes": gate_sizes,
        "valid_promotion": promoted["phase"],
    }, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
