"""Repository-local governance gate for the isolated minimal target PoC."""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import sys
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any


INVALID = 2
HALTED = 3


class GateError(Exception):
    pass


def load_json(path: Path) -> dict[str, Any]:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise GateError(f"cannot read valid JSON: {path}: {exc}") from exc


def resolve(project: Path, value: str) -> Path:
    path = Path(value)
    return path if path.is_absolute() else project / path


def local_artifact(project: Path, value: str) -> Path:
    require(isinstance(value, str) and value, "stdout_file is required")
    project = project.resolve()
    path = resolve(project, value).resolve()
    try:
        path.relative_to(project)
    except ValueError as exc:
        raise GateError("stdout_file must stay inside the target project") from exc
    require(path.is_file(), f"stdout_file does not exist: {value}")
    return path


def canonical_stdout_sha256(path: Path) -> str:
    data = path.read_bytes().replace(b"\r\n", b"\n").replace(b"\r", b"\n")
    return hashlib.sha256(data).hexdigest()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise GateError(message)


def governance(project: Path) -> dict[str, Any]:
    data = load_json(project / ".ai-governance" / "governance.json")
    phase = data.get("phase")
    require(isinstance(phase, int) and 0 <= phase <= 4, "phase must be an integer from 0 to 4")
    return data


def gates_for_phase(phase: int) -> list[str]:
    gates = ["task_contract", "lifecycle_awareness"]
    if phase >= 1:
        gates += ["pre_implementation_search", "scoped_change", "command_evidence"]
    if phase >= 2:
        gates += ["independent_review"]
    if phase >= 3:
        gates += ["hashed_stdout", "state_gate"]
    if phase >= 4:
        gates += ["total_failure_cap"]
    return gates


def maturity(project: Path) -> dict[str, Any]:
    return load_json(project / ".ai-governance" / "maturity.json")


def trust(project: Path) -> dict[str, Any]:
    return load_json(project / ".ai-governance" / "trust.json")


def assess_phase(data: dict[str, Any]) -> dict[str, Any]:
    recommended = 0
    triggers: list[str] = []
    if not data.get("core_hypothesis_proven", False):
        return {"recommended_phase": recommended, "triggers": triggers}
    recommended = 1
    triggers.append("core_hypothesis_proven")
    phase2 = []
    if data.get("first_external_user", False):
        phase2.append("first_external_user")
    if int(data.get("code_lines", 0)) >= 20000:
        phase2.append("code_lines>=20000")
    if int(data.get("module_count", 0)) > 5:
        phase2.append("module_count>5")
    if data.get("first_production_incident", False):
        phase2.append("first_production_incident")
    if not phase2:
        return {"recommended_phase": recommended, "triggers": triggers}
    recommended = 2
    triggers.extend(phase2)
    phase3 = []
    if int(data.get("team_size", 1)) > 1:
        phase3.append("team_size>1")
    if int(data.get("parallel_agent_sessions", 1)) > 3:
        phase3.append("parallel_agent_sessions>3")
    if int(data.get("max_module_reuse", 0)) >= 3:
        phase3.append("max_module_reuse>=3")
    if data.get("compliance_requirement", False):
        phase3.append("compliance_requirement")
    if not phase3:
        return {"recommended_phase": recommended, "triggers": triggers}
    recommended = 3
    triggers.extend(phase3)
    if data.get("long_term_sla_module", False):
        recommended = 4
        triggers.append("long_term_sla_module")
    return {"recommended_phase": recommended, "triggers": triggers}


def state_path(project: Path) -> Path:
    return project / ".ai-governance" / "state.json"


def load_state(project: Path) -> dict[str, Any]:
    path = state_path(project)
    return load_json(path) if path.exists() else {"tasks": {}}


def save_state(project: Path, state: dict[str, Any]) -> None:
    path = state_path(project)
    path.write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def task_state(project: Path, task_id: str) -> dict[str, Any]:
    return load_state(project).get("tasks", {}).get(task_id, {})


def validate_task(data: dict[str, Any], configured_phase: int) -> None:
    require(isinstance(data.get("task_id"), str) and data["task_id"], "task_id is required")
    require(data.get("phase") == configured_phase, "task phase must match project phase")
    criteria = data.get("acceptance_criteria")
    require(isinstance(criteria, list) and criteria and all(isinstance(x, str) and x for x in criteria),
            "acceptance_criteria must be a non-empty string list")
    allowed = data.get("allowed_files")
    require(isinstance(allowed, list) and allowed and all(isinstance(x, str) and x for x in allowed),
            "allowed_files must be a non-empty string list")
    search = data.get("pre_implementation_search")
    require(isinstance(search, dict), "pre_implementation_search is required")
    commands = search.get("commands")
    require(isinstance(commands, list) and commands and all(isinstance(x, str) and x for x in commands),
            "pre_implementation_search.commands must be a non-empty string list")
    require(isinstance(search.get("evidence"), str) and search["evidence"],
            "pre_implementation_search.evidence is required")
    require(isinstance(data.get("author_agent_id"), str) and data["author_agent_id"], "author_agent_id is required")
    require(isinstance(data.get("author_session_id"), str) and data["author_session_id"], "author_session_id is required")


def normalized_relative(path: str) -> str:
    require(isinstance(path, str) and path, "file path must be a non-empty string")
    candidate = PurePosixPath(path.replace("\\", "/"))
    require(not candidate.is_absolute(), "file path must be project-relative")
    require(".." not in candidate.parts, "file path must not contain parent traversal")
    require(not (candidate.parts and candidate.parts[0].endswith(":")), "file path must not contain a drive prefix")
    normalized = candidate.as_posix()
    return normalized[2:] if normalized.startswith("./") else normalized


def path_allowed(path: str, allowed: list[str]) -> bool:
    normalized = normalized_relative(path)
    for entry in allowed:
        candidate = normalized_relative(entry)
        if normalized == candidate or normalized.startswith(candidate.rstrip("/") + "/"):
            return True
    return False


def validate_evidence(project: Path, task: dict[str, Any], evidence: dict[str, Any], phase: int) -> None:
    require(evidence.get("task_id") == task["task_id"], "evidence task_id mismatch")
    changed = evidence.get("changed_files")
    require(isinstance(changed, list), "changed_files must be a list")
    require(all(path_allowed(path, task["allowed_files"]) for path in changed),
            "changed_files contains a path outside allowed_files")
    commands = evidence.get("commands")
    require(isinstance(commands, list) and commands, "at least one evidence command is required")
    for item in commands:
        require(isinstance(item, dict) and isinstance(item.get("cmd"), str) and item["cmd"],
                "every evidence command needs cmd")
        require(item.get("exit_code") == 0, "every evidence command must have exit_code 0")
        if phase >= 3:
            digest = item.get("stdout_sha256")
            require(isinstance(digest, str) and len(digest) == 64 and all(c in "0123456789abcdef" for c in digest.lower()),
                    "phase 3+ commands require a 64-character stdout_sha256")
            actual = canonical_stdout_sha256(local_artifact(project, item.get("stdout_file")))
            require(actual == digest.lower(), "stdout_sha256 does not match stdout_file")
    checklist = evidence.get("acceptance_checklist")
    require(isinstance(checklist, list), "acceptance_checklist must be a list")
    by_criterion = {item.get("criterion"): item for item in checklist if isinstance(item, dict)}
    for criterion in task["acceptance_criteria"]:
        item = by_criterion.get(criterion)
        require(bool(item) and item.get("verified") is True and isinstance(item.get("evidence_ref"), str)
                and item["evidence_ref"], f"acceptance criterion not verified: {criterion}")


def validate_review(project: Path, task: dict[str, Any], evidence: dict[str, Any], review: dict[str, Any], phase: int) -> None:
    if phase < 2:
        return
    require(review.get("task_id") == task["task_id"], "review task_id mismatch")
    require(review.get("reviewer_agent_id") != task["author_agent_id"], "reviewer Agent must differ from author")
    require(review.get("reviewer_session_id") != task["author_session_id"], "reviewer session must differ from author session")
    require(review.get("decision") == "PASS", "review decision must be PASS")
    require(isinstance(review.get("pre_mortem"), str) and review["pre_mortem"], "review pre_mortem is required")
    require(isinstance(review.get("commands"), list) and review["commands"], "reviewer must independently run a command")
    require(all(isinstance(item, dict) and item.get("exit_code") == 0 for item in review["commands"]),
            "every reviewer command must have exit_code 0")
    if phase >= 3:
        reviewer_by_cmd = {item.get("cmd"): item for item in review["commands"]}
        for executor_item in evidence["commands"]:
            reviewer_item = reviewer_by_cmd.get(executor_item["cmd"])
            require(bool(reviewer_item), f"reviewer did not rerun command: {executor_item['cmd']}")
            actual = canonical_stdout_sha256(local_artifact(project, reviewer_item.get("stdout_file")))
            require(reviewer_item.get("stdout_sha256") == actual, "reviewer stdout_sha256 does not match stdout_file")
            require(actual == executor_item.get("stdout_sha256"), "reviewer output differs from executor evidence")


def ensure_not_halted(project: Path, task_id: str) -> None:
    require(task_state(project, task_id).get("status") != "HALTED_FOR_HUMAN",
            f"task {task_id} is HALTED_FOR_HUMAN")


def inspect_command(args: argparse.Namespace) -> int:
    project = Path(args.project).resolve()
    data = governance(project)
    print(json.dumps({"phase": data["phase"], "gates": gates_for_phase(data["phase"])}, ensure_ascii=False, indent=2))
    return 0


def assess_phase_command(args: argparse.Namespace) -> int:
    project = Path(args.project).resolve()
    current = governance(project)["phase"]
    assessment = assess_phase(maturity(project))
    print(json.dumps({"current_phase": current, **assessment}, ensure_ascii=False, indent=2))
    return 0


def promote_command(args: argparse.Namespace) -> int:
    project = Path(args.project).resolve()
    config_path = project / ".ai-governance" / "governance.json"
    config = governance(project)
    current = config["phase"]
    require(current < 4, "phase 4 is already the maximum")
    assessment = assess_phase(maturity(project))
    require(assessment["recommended_phase"] > current,
            f"maturity signals do not permit promotion beyond phase {current}")
    target = current + 1
    transition = f"{current}->{target}"
    approval_path = local_artifact(project, args.approval_file)
    approval = load_json(approval_path)
    require(approval.get("from_phase") == current and approval.get("to_phase") == target,
            f"approval must be bound to transition {transition}")
    require(isinstance(approval.get("approved_by"), str) and approval["approved_by"].strip(),
            "approval approved_by is required")
    token = approval.get("token")
    require(isinstance(token, str) and token, "approval token is required")
    expected = trust(project).get("promotion_token_sha256", {}).get(transition)
    require(isinstance(expected, str) and len(expected) == 64,
            f"no trusted approval token is configured for transition {transition}")
    actual = hashlib.sha256(token.encode("utf-8")).hexdigest()
    require(hmac.compare_digest(actual, expected.lower()), "approval token does not match the local trust anchor")
    approval_hash = hashlib.sha256(approval_path.read_bytes()).hexdigest()
    config["phase"] = target
    history = config.setdefault("phase_history", [])
    history.append({
        "from": current,
        "to": target,
        "approved_by": approval["approved_by"].strip(),
        "approval_sha256": approval_hash,
        "triggers": assessment["triggers"],
        "timestamp": datetime.now(timezone.utc).isoformat(),
    })
    config_path.write_text(json.dumps(config, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"from": current, "to": target, "gates": gates_for_phase(target)}, ensure_ascii=False))
    return 0


def audit_command(args: argparse.Namespace) -> int:
    project = Path(args.project).resolve()
    phase = governance(project)["phase"]
    task = load_json(resolve(project, args.task))
    validate_task(task, phase)
    ensure_not_halted(project, task["task_id"])
    evidence = load_json(resolve(project, args.evidence))
    validate_evidence(project, task, evidence, phase)
    if phase >= 2:
        require(bool(args.review), "phase 2+ audit requires --review")
        validate_review(project, task, evidence, load_json(resolve(project, args.review)), phase)
    print(json.dumps({"task_id": task["task_id"], "result": "PASS", "phase": phase}, ensure_ascii=False))
    return 0


def record_attempt_command(args: argparse.Namespace) -> int:
    project = Path(args.project).resolve()
    config = governance(project)
    state = load_state(project)
    tasks = state.setdefault("tasks", {})
    current = tasks.setdefault(args.task_id, {"status": "EXECUTING", "consecutive_signature": None,
                                               "consecutive_count": 0, "total_failures": 0})
    if current.get("status") == "HALTED_FOR_HUMAN":
        print(f"task {args.task_id} is already HALTED_FOR_HUMAN", file=sys.stderr)
        return HALTED
    if args.exit_code == 0:
        current.update({"consecutive_signature": None, "consecutive_count": 0})
    else:
        require(bool(args.signature), "failed attempt requires --signature")
        current["total_failures"] = int(current.get("total_failures", 0)) + 1
        if current.get("consecutive_signature") == args.signature:
            current["consecutive_count"] = int(current.get("consecutive_count", 0)) + 1
        else:
            current["consecutive_signature"] = args.signature
            current["consecutive_count"] = 1
        same_limit = config["failure_policy"]["same_signature_limit"]
        total_limit = config["failure_policy"]["total_failure_limit_phase4"]
        if current["consecutive_count"] >= same_limit or (config["phase"] >= 4 and current["total_failures"] >= total_limit):
            current["status"] = "HALTED_FOR_HUMAN"
    save_state(project, state)
    print(json.dumps(current, ensure_ascii=False))
    return HALTED if current.get("status") == "HALTED_FOR_HUMAN" else 0


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description=__doc__)
    sub = root.add_subparsers(dest="command", required=True)
    inspect = sub.add_parser("inspect")
    inspect.add_argument("--project", required=True)
    inspect.set_defaults(func=inspect_command)
    assess = sub.add_parser("assess-phase")
    assess.add_argument("--project", required=True)
    assess.set_defaults(func=assess_phase_command)
    promote = sub.add_parser("promote")
    promote.add_argument("--project", required=True)
    promote.add_argument("--approval-file", required=True)
    promote.set_defaults(func=promote_command)
    audit = sub.add_parser("audit")
    audit.add_argument("--project", required=True)
    audit.add_argument("--task", required=True)
    audit.add_argument("--evidence", required=True)
    audit.add_argument("--review")
    audit.set_defaults(func=audit_command)
    attempt = sub.add_parser("record-attempt")
    attempt.add_argument("--project", required=True)
    attempt.add_argument("--task-id", required=True)
    attempt.add_argument("--signature")
    attempt.add_argument("--exit-code", required=True, type=int)
    attempt.set_defaults(func=record_attempt_command)
    return root


def main() -> int:
    args = parser().parse_args()
    try:
        return args.func(args)
    except GateError as exc:
        print(f"governance gate failed: {exc}", file=sys.stderr)
        return INVALID


if __name__ == "__main__":
    raise SystemExit(main())
