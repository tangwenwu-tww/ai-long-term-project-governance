import importlib.util
import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TARGET = ROOT / "poc" / "minimal-target"
GATE_PATH = TARGET / ".ai-governance" / "gate.py"
SPEC = importlib.util.spec_from_file_location("local_gate", GATE_PATH)
gate = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(gate)


class GovernanceGateTests(unittest.TestCase):
    def setUp(self):
        self.config = gate.load_json(TARGET / ".ai-governance" / "governance.json")
        self.task = gate.load_json(TARGET / "tasks" / "T-001.json")
        self.evidence = gate.load_json(TARGET / "evidence" / "T-001.json")
        self.review = gate.load_json(TARGET / "reviews" / "T-001.json")

    def test_phase_progressively_adds_gates(self):
        previous = set()
        for phase in range(5):
            current = set(gate.gates_for_phase(phase))
            self.assertTrue(previous.issubset(current))
            previous = current
        self.assertNotIn("independent_review", gate.gates_for_phase(1))
        self.assertIn("independent_review", gate.gates_for_phase(2))
        self.assertIn("hashed_stdout", gate.gates_for_phase(3))
        self.assertIn("total_failure_cap", gate.gates_for_phase(4))

    def test_maturity_signals_recommend_phases_zero_through_four(self):
        signals = {}
        self.assertEqual(gate.assess_phase(signals)["recommended_phase"], 0)
        signals["core_hypothesis_proven"] = True
        self.assertEqual(gate.assess_phase(signals)["recommended_phase"], 1)
        signals["first_external_user"] = True
        self.assertEqual(gate.assess_phase(signals)["recommended_phase"], 2)
        signals["team_size"] = 2
        self.assertEqual(gate.assess_phase(signals)["recommended_phase"], 3)
        signals["long_term_sla_module"] = True
        self.assertEqual(gate.assess_phase(signals)["recommended_phase"], 4)

    def test_promotion_is_incremental_and_strengthens_gates(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            config = json.loads(json.dumps(self.config))
            config["phase"] = 0
            (governance_dir / "governance.json").write_text(json.dumps(config), encoding="utf-8")
            signals = {
                "core_hypothesis_proven": True,
                "first_external_user": True,
                "team_size": 2,
                "long_term_sla_module": True,
            }
            (governance_dir / "maturity.json").write_text(json.dumps(signals), encoding="utf-8")

            tokens = {f"{phase}->{phase + 1}": f"human-token-{phase}" for phase in range(4)}
            trust = {"promotion_token_sha256": {
                transition: gate.hashlib.sha256(token.encode("utf-8")).hexdigest()
                for transition, token in tokens.items()
            }}
            (governance_dir / "trust.json").write_text(json.dumps(trust), encoding="utf-8")

            class Args:
                pass

            args = Args()
            args.project = str(project)
            for expected in range(1, 5):
                approval = {
                    "from_phase": expected - 1,
                    "to_phase": expected,
                    "approved_by": "human-owner",
                    "token": tokens[f"{expected - 1}->{expected}"],
                }
                approval_path = project / f"approval-{expected}.json"
                approval_path.write_text(json.dumps(approval), encoding="utf-8")
                args.approval_file = str(approval_path)
                self.assertEqual(gate.promote_command(args), 0)
                updated = gate.governance(project)
                self.assertEqual(updated["phase"], expected)
                self.assertEqual(updated["phase_history"][-1]["to"], expected)
            self.assertIn("total_failure_cap", gate.gates_for_phase(gate.governance(project)["phase"]))

    def test_promotion_fails_without_maturity_signal(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            config = json.loads(json.dumps(self.config))
            config["phase"] = 1
            (governance_dir / "governance.json").write_text(json.dumps(config), encoding="utf-8")
            (governance_dir / "maturity.json").write_text(
                json.dumps({"core_hypothesis_proven": True}), encoding="utf-8")
            (governance_dir / "trust.json").write_text(
                json.dumps({"promotion_token_sha256": {}}), encoding="utf-8")

            class Args:
                approval_file = str(project / "missing-approval.json")

            args = Args()
            args.project = str(project)
            with self.assertRaises(gate.GateError):
                gate.promote_command(args)

    def test_promotion_rejects_forged_or_wrong_transition_approval(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            config = json.loads(json.dumps(self.config))
            config["phase"] = 1
            (governance_dir / "governance.json").write_text(json.dumps(config), encoding="utf-8")
            signals = {"core_hypothesis_proven": True, "first_external_user": True}
            (governance_dir / "maturity.json").write_text(json.dumps(signals), encoding="utf-8")
            trusted_token = "issued-by-human"
            trust = {"promotion_token_sha256": {
                "1->2": gate.hashlib.sha256(trusted_token.encode("utf-8")).hexdigest()
            }}
            (governance_dir / "trust.json").write_text(json.dumps(trust), encoding="utf-8")

            class Args:
                pass

            args = Args()
            args.project = str(project)
            approval_path = project / "approval.json"
            args.approval_file = str(approval_path)
            forged = {"from_phase": 1, "to_phase": 2, "approved_by": "agent", "token": "forged"}
            approval_path.write_text(json.dumps(forged), encoding="utf-8")
            with self.assertRaises(gate.GateError):
                gate.promote_command(args)
            wrong_transition = {"from_phase": 0, "to_phase": 1, "approved_by": "human", "token": trusted_token}
            approval_path.write_text(json.dumps(wrong_transition), encoding="utf-8")
            with self.assertRaises(gate.GateError):
                gate.promote_command(args)

    def test_valid_contract_evidence_and_review_pass(self):
        gate.validate_task(self.task, 3)
        gate.validate_evidence(TARGET, self.task, self.evidence, 3)
        gate.validate_review(TARGET, self.task, self.evidence, self.review, 3)

    def test_task_without_acceptance_criteria_fails(self):
        invalid = dict(self.task)
        invalid["acceptance_criteria"] = []
        with self.assertRaises(gate.GateError):
            gate.validate_task(invalid, 3)

    def test_out_of_scope_change_fails(self):
        invalid = json.loads(json.dumps(self.evidence))
        invalid["changed_files"].append("src/outside.py")
        with self.assertRaises(gate.GateError):
            gate.validate_evidence(TARGET, self.task, invalid, 3)

    def test_path_traversal_cannot_bypass_allowed_files(self):
        for path in ("../artifacts/result.txt", "artifacts/../outside.txt", "C:/artifacts/result.txt"):
            invalid = json.loads(json.dumps(self.evidence))
            invalid["changed_files"] = [path]
            with self.subTest(path=path), self.assertRaises(gate.GateError):
                gate.validate_evidence(TARGET, self.task, invalid, 3)

    def test_phase3_requires_stdout_hash(self):
        invalid = json.loads(json.dumps(self.evidence))
        del invalid["commands"][0]["stdout_sha256"]
        with self.assertRaises(gate.GateError):
            gate.validate_evidence(TARGET, self.task, invalid, 3)
        gate.validate_evidence(TARGET, self.task, invalid, 2)

    def test_phase3_recomputes_stdout_hash_and_rejects_escape(self):
        invalid_hash = json.loads(json.dumps(self.evidence))
        invalid_hash["commands"][0]["stdout_sha256"] = "0" * 64
        with self.assertRaises(gate.GateError):
            gate.validate_evidence(TARGET, self.task, invalid_hash, 3)
        escaped = json.loads(json.dumps(self.evidence))
        escaped["commands"][0]["stdout_file"] = "../outside.stdout"
        with self.assertRaises(gate.GateError):
            gate.validate_evidence(TARGET, self.task, escaped, 3)

    def test_review_must_use_independent_agent_and_session(self):
        same_agent = dict(self.review, reviewer_agent_id=self.task["author_agent_id"])
        with self.assertRaises(gate.GateError):
            gate.validate_review(TARGET, self.task, self.evidence, same_agent, 3)
        same_session = dict(self.review, reviewer_session_id=self.task["author_session_id"])
        with self.assertRaises(gate.GateError):
            gate.validate_review(TARGET, self.task, self.evidence, same_session, 3)

    def test_reviewer_must_match_executor_output(self):
        mismatch = json.loads(json.dumps(self.review))
        mismatch["commands"][0]["stdout_sha256"] = "0" * 64
        with self.assertRaises(gate.GateError):
            gate.validate_review(TARGET, self.task, self.evidence, mismatch, 3)

    def test_same_failure_signature_halts_on_third_attempt_and_success_resets(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            (governance_dir / "governance.json").write_text(json.dumps(self.config), encoding="utf-8")

            class Args:
                task_id = "T-FAIL"
                signature = "AssertionError: expected 400 got 500"
                exit_code = 1

            args = Args()
            args.project = str(project)
            self.assertEqual(gate.record_attempt_command(args), 0)
            args.exit_code = 0
            self.assertEqual(gate.record_attempt_command(args), 0)
            args.exit_code = 1
            self.assertEqual(gate.record_attempt_command(args), 0)
            self.assertEqual(gate.record_attempt_command(args), 0)
            self.assertEqual(gate.record_attempt_command(args), gate.HALTED)
            state = gate.load_state(project)["tasks"][args.task_id]
            self.assertEqual(state["status"], "HALTED_FOR_HUMAN")
            self.assertEqual(state["consecutive_count"], 3)

    def test_phase4_halts_after_ten_total_failures_across_signatures(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            config = json.loads(json.dumps(self.config))
            config["phase"] = 4
            (governance_dir / "governance.json").write_text(json.dumps(config), encoding="utf-8")

            class Args:
                task_id = "T-PHASE4"
                exit_code = 1

            args = Args()
            args.project = str(project)
            for attempt in range(1, 10):
                args.signature = f"failure-{attempt}"
                self.assertEqual(gate.record_attempt_command(args), 0)
            args.signature = "failure-10"
            self.assertEqual(gate.record_attempt_command(args), gate.HALTED)
            state = gate.load_state(project)["tasks"][args.task_id]
            self.assertEqual(state["total_failures"], 10)
            self.assertEqual(state["status"], "HALTED_FOR_HUMAN")

    def test_halted_task_cannot_continue_audit(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp)
            governance_dir = project / ".ai-governance"
            governance_dir.mkdir()
            (governance_dir / "state.json").write_text(
                json.dumps({"tasks": {"T-001": {"status": "HALTED_FOR_HUMAN"}}}), encoding="utf-8")
            with self.assertRaises(gate.GateError):
                gate.ensure_not_halted(project, "T-001")

    def test_copied_target_runs_using_only_local_governance(self):
        with tempfile.TemporaryDirectory() as temp:
            copied = Path(temp) / "isolated-target"
            shutil.copytree(TARGET, copied)
            local_gate = copied / ".ai-governance" / "gate.py"
            inspect = subprocess.run(
                [sys.executable, str(local_gate), "inspect", "--project", str(copied)],
                cwd=copied, capture_output=True, text=True, check=False)
            self.assertEqual(inspect.returncode, 0, inspect.stderr)
            self.assertEqual(json.loads(inspect.stdout)["phase"], 3)
            audit = subprocess.run(
                [sys.executable, str(local_gate), "audit", "--project", str(copied),
                 "--task", "tasks/T-001.json", "--evidence", "evidence/T-001.json",
                 "--review", "reviews/T-001.json"],
                cwd=copied, capture_output=True, text=True, check=False)
            self.assertEqual(audit.returncode, 0, audit.stderr)
            self.assertEqual(json.loads(audit.stdout)["result"], "PASS")


if __name__ == "__main__":
    unittest.main()
