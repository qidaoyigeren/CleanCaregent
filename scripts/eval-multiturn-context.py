#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


def request_json(method, base_url, path, payload=None, timeout=120):
    data = None
    headers = {"Content-Type": "application/json; charset=utf-8"}
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        urllib.parse.urljoin(base_url.rstrip("/") + "/", path.lstrip("/")),
        data=data,
        method=method,
        headers=headers,
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode("utf-8")
            return response.status, json.loads(body)
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {path} returned {error.code}: {body}") from error


def load_cases(path):
    with open(path, "r", encoding="utf-8") as source:
        cases = json.load(source)
    if not isinstance(cases, list):
        raise ValueError("multi-turn cases file must contain a JSON array")
    for index, item in enumerate(cases):
        if not item.get("case_id") or not item.get("turns"):
            raise ValueError(f"case at index {index} is missing case_id or turns")
    return cases


def envelope_data(envelope):
    if envelope.get("code") != "OK":
        raise RuntimeError(f"unexpected API envelope: {envelope}")
    return envelope.get("data") or {}


def fetch_trace(base_url, trace_id, request_timeout):
    last_error = None
    for _ in range(12):
        try:
            _, trace_envelope = request_json(
                "GET",
                base_url,
                f"/api/v1/admin/traces/{trace_id}",
                timeout=request_timeout,
            )
            return envelope_data(trace_envelope)
        except RuntimeError as error:
            last_error = error
            if "TRACE_NOT_FOUND" not in str(error):
                raise
            time.sleep(1)
    raise RuntimeError(f"trace {trace_id} was not available: {last_error}")


def get_nested(value, *keys):
    current = value
    for key in keys:
        if not isinstance(current, dict):
            return None
        current = current.get(key)
    return current


def iter_plan_steps(trace_data):
    plan = trace_data.get("plan") or {}
    for key in ("Steps", "steps"):
        steps = plan.get(key)
        if isinstance(steps, list):
            for step in steps:
                if isinstance(step, dict):
                    yield step


def collect_plan_context(trace_data, tool_calls=None):
    queries = []
    params = {}
    for step in iter_plan_steps(trace_data):
        query = step.get("Query") or step.get("query")
        if query:
            queries.append(str(query))
        step_params = step.get("Params") or step.get("params")
        if isinstance(step_params, dict):
            params.update({str(k): str(v) for k, v in step_params.items()})
    for step in trace_data.get("steps") or []:
        metadata = step.get("metadata") or {}
        input_data = metadata.get("input") or {}
        query = input_data.get("query")
        if query:
            queries.append(str(query))
        step_params = input_data.get("params")
        if isinstance(step_params, dict):
            params.update({str(k): str(v) for k, v in step_params.items()})
    for call in tool_calls or []:
        arguments = call.get("arguments") or {}
        if isinstance(arguments, dict):
            params.update({str(k): str(v) for k, v in arguments.items() if v not in (None, "", "<nil>")})
    return " ".join(queries), params


STATE_CHANGE_TOOLS = {
    "create_after_sales_ticket",
    "return_request",
    "exchange_request",
    "handoff_to_human",
}

PHONE_RE = re.compile(r"(?:\+?86[-\s]?)?1[3-9][0-9][-\s]?[0-9]{4}[-\s]?[0-9]{4}")
ADDRESS_RE = re.compile(r"(?:省|市|区|县|街道|小区|单元|门牌|address).{0,20}[0-9]{1,4}(?:号|室|room)?", re.I)
ORDER_ID_RE = re.compile(r"(?i)^CC[0-9]{6,}$")


def logical_tool_name(value):
    value = str(value or "").strip().lower()
    if "/" in value:
        return value.rsplit("/", 1)[-1]
    return value


def masked_identifier(value):
    value = str(value or "").strip()
    if not value:
        return ""
    if ORDER_ID_RE.match(value) and len(value) > 6:
        return value[:2] + "****" + value[-4:]
    return value


def identifier_matches(actual, expected):
    actual = str(actual or "").strip()
    expected = str(expected or "").strip()
    if actual == expected:
        return True
    if masked_identifier(expected) == actual:
        return True
    if masked_identifier(actual) == expected:
        return True
    return False


def answer_contains_required(answer, term):
    if term in answer:
        return True
    masked = masked_identifier(term)
    return masked != term and masked in answer


def collect_tool_calls(trace_data):
    calls = []
    for raw in trace_data.get("tool_calls") or trace_data.get("ToolCalls") or []:
        if not isinstance(raw, dict):
            continue
        name = (
            raw.get("tool_name")
            or raw.get("ToolName")
            or raw.get("name")
            or raw.get("Name")
        )
        if not name:
            continue
        calls.append({
            "name": logical_tool_name(name),
            "raw_name": name,
            "status": raw.get("status") or raw.get("Status") or "",
            "error_code": raw.get("error_code") or raw.get("ErrorCode") or "",
            "arguments": raw.get("arguments") or raw.get("Arguments") or {},
            "result": raw.get("result_summary") or raw.get("ResultSummary") or {},
        })
    return calls


SENSITIVE_FIELD_NAMES = {
    "user_id", "user_no", "phone", "mobile", "contact", "address",
    "手机号", "联系电话", "详细地址",
}


def redacted_value(value):
    if value is None:
        return True
    text = str(value).strip().lower()
    return text in {"", "[redacted]", "<redacted>", "***", "null"}


def contains_pii_value(value):
    if isinstance(value, dict):
        for key, item in value.items():
            key_text = str(key).lower()
            if key_text in SENSITIVE_FIELD_NAMES:
                if not redacted_value(item):
                    return True
                continue
            if contains_pii_value(item):
                return True
        return False
    if isinstance(value, list):
        return any(contains_pii_value(item) for item in value)
    text = str(value)
    if redacted_value(text):
        return False
    return bool(PHONE_RE.search(text) or ADDRESS_RE.search(text))


def contains_pii(*values):
    return any(contains_pii_value(value) for value in values)


def forbidden_phrase_present(answer, phrase):
    index = answer.find(phrase)
    while index >= 0:
        prefix = answer[max(0, index - 12):index]
        if not any(marker in prefix for marker in ("无法确认", "不能确认", "未确认", "没有", "不会", "不能", "无法", "并未")):
            return True
        index = answer.find(phrase, index + len(phrase))
    return False

def run_case(base_url, case, request_timeout):
    started = time.monotonic()
    status, envelope = request_json(
        "POST",
        base_url,
        "/api/v1/conversations",
        {"title": "multiturn eval " + case["case_id"]},
        timeout=request_timeout,
    )
    if status != 201:
        raise RuntimeError(f"create conversation returned {status}: {envelope}")
    conversation_id = envelope_data(envelope)["conversation_id"]

    turn_records = []
    final_response = None
    for turn in case["turns"]:
        _, envelope = request_json(
            "POST",
            base_url,
            f"/api/v1/conversations/{conversation_id}/messages",
            {"content": turn},
            timeout=request_timeout,
        )
        final_response = envelope_data(envelope)
        turn_records.append({
            "turn": turn,
            "trace_id": final_response.get("trace_id"),
            "answer_preview": (final_response.get("answer") or "")[:180],
        })

    trace_id = final_response.get("trace_id")
    expected_intent = case.get("expected_intent")
    try:
        trace_data = fetch_trace(base_url, trace_id, request_timeout)
    except RuntimeError as error:
        if expected_intent != "out_of_scope":
            raise
        answer = final_response.get("answer") or ""
        failures = []
        for phrase in case.get("forbidden_answer_terms") or []:
            if forbidden_phrase_present(answer, phrase):
                failures.append(f"forbidden phrase present {phrase!r}")
        pii_leak = contains_pii(answer)
        if case.get("forbid_pii", True) and pii_leak:
            failures.append("PII leak detected")
        return {
            "case_id": case["case_id"],
            "passed": not failures,
            "failures": failures,
            "conversation_id": conversation_id,
            "final_trace_id": trace_id,
            "actual_intent": "out_of_scope",
            "plan_query": "",
            "plan_params": {},
            "tool_calls": [],
            "tool_names": [],
            "tool_accuracy": True,
            "false_action": False,
            "pii_leak": pii_leak,
            "input_tokens": 0,
            "output_tokens": 0,
            "total_tokens": 0,
            "latency_ms": int((time.monotonic() - started) * 1000),
            "turns": turn_records,
            "answer_preview": answer[:500],
            "trace_unavailable": str(error),
        }
    answer = final_response.get("answer") or ""
    tool_calls = collect_tool_calls(trace_data)
    plan_query, plan_params = collect_plan_context(trace_data, tool_calls)
    tool_names = [item["name"] for item in tool_calls]
    input_tokens = int(trace_data.get("input_tokens") or 0)
    output_tokens = int(trace_data.get("output_tokens") or 0)
    answer_haystack = answer

    failures = []
    actual_intent = trace_data.get("intent")
    if expected_intent and actual_intent != expected_intent:
        failures.append(f"intent {actual_intent!r} != {expected_intent!r}")
    for key, expected in (case.get("expected_entities") or {}).items():
        actual = str(plan_params.get(key, ""))
        if not identifier_matches(actual, expected):
            failures.append(f"entity {key} {actual!r} != {expected!r}")
    for term in case.get("required_query_terms") or []:
        if term not in plan_query:
            failures.append(f"plan query missing {term!r}")
    for term in case.get("required_answer_terms") or []:
        if not answer_contains_required(answer, term):
            failures.append(f"answer missing {term!r}")
    for phrase in case.get("forbidden_answer_terms") or []:
        if forbidden_phrase_present(answer_haystack, phrase):
            failures.append(f"forbidden phrase present {phrase!r}")
    expected_tools = [logical_tool_name(item) for item in case.get("expected_tools") or []]
    forbidden_tools = [logical_tool_name(item) for item in case.get("forbidden_tools") or []]
    for expected in expected_tools:
        if expected not in tool_names:
            failures.append(f"tool missing {expected!r}")
    for forbidden in forbidden_tools:
        if forbidden in tool_names:
            failures.append(f"forbidden tool called {forbidden!r}")
    false_action = any(name in STATE_CHANGE_TOOLS for name in tool_names)
    if expected_tools:
        false_action = any(name in STATE_CHANGE_TOOLS and name not in expected_tools for name in tool_names)
    if case.get("forbid_state_change") and any(name in STATE_CHANGE_TOOLS for name in tool_names):
        false_action = True
    if false_action:
        failures.append("state-changing tool was called without expectation")
    pii_leak = contains_pii(answer, [item.get("result") for item in tool_calls])
    if case.get("forbid_pii", True) and pii_leak:
        failures.append("PII leak detected")
    if case.get("require_tokens", True) and input_tokens + output_tokens <= 0:
        failures.append("trace has no token usage")
    tool_accuracy = not any(
        failure.startswith("tool missing") or failure.startswith("forbidden tool")
        for failure in failures
    )

    return {
        "case_id": case["case_id"],
        "passed": not failures,
        "failures": failures,
        "conversation_id": conversation_id,
        "final_trace_id": trace_id,
        "actual_intent": actual_intent,
        "plan_query": plan_query,
        "plan_params": plan_params,
        "tool_calls": tool_calls,
        "tool_names": tool_names,
        "tool_accuracy": tool_accuracy,
        "false_action": false_action,
        "pii_leak": pii_leak,
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
        "total_tokens": input_tokens + output_tokens,
        "latency_ms": int((time.monotonic() - started) * 1000),
        "turns": turn_records,
        "answer_preview": answer[:500],
    }


def write_markdown(path, report):
    lines = [
        "# Multi-turn Context Evaluation",
        "",
        f"Generated at: {report['generated_at']}",
        f"Run ID: `{report['run_id']}`",
        f"Base URL: `{report['base_url']}`",
        "",
        "## Summary",
        "",
        "| Metric | Value |",
        "|---|---:|",
        f"| Total Cases | {report['total_cases']} |",
        f"| Passed Cases | {report['passed_cases']} |",
        f"| Pass Rate | {report['pass_rate'] * 100:.1f}% |",
        f"| Tool Accuracy | {report['tool_accuracy'] * 100:.1f}% |",
        f"| False Action Rate | {report['false_action_rate'] * 100:.1f}% |",
        f"| PII Leak Rate | {report['pii_leak_rate'] * 100:.1f}% |",
        f"| Average Tokens | {report['average_tokens']:.2f} |",
        f"| P95 Latency | {report['p95_latency_ms']} ms |",
        "",
        "## Failures",
        "",
        "| Case | Intent | Trace | Failures |",
        "|---|---|---|---|",
    ]
    failures = [item for item in report["results"] if not item["passed"]]
    if not failures:
        lines.append("| none | - | - | - |")
    for item in failures:
        lines.append(
            f"| `{item['case_id']}` | `{item.get('actual_intent') or ''}` | "
            f"`{item.get('final_trace_id') or ''}` | {'; '.join(item['failures'])} |"
        )
    lines.append("")
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "w", encoding="utf-8") as target:
        target.write("\n".join(lines))


def percentile95(values):
    if not values:
        return 0
    ordered = sorted(values)
    index = (95 * len(ordered) + 99) // 100 - 1
    return ordered[max(0, min(index, len(ordered) - 1))]


def parse_args():
    parser = argparse.ArgumentParser(description="Run real HTTP multi-turn context evaluation.")
    parser.add_argument("--base-url", default=os.environ.get("BASE_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--cases", default="docs/eval/multiturn-cases-v1.json")
    parser.add_argument("--output", default="docs/eval/multiturn-context-report.md")
    parser.add_argument("--json-output", default=".e2e/multiturn-context-result.json")
    parser.add_argument("--request-timeout", type=float, default=180)
    return parser.parse_args()


def main():
    args = parse_args()
    cases = load_cases(args.cases)
    results = []
    for index, case in enumerate(cases, 1):
        print(f"[{index}/{len(cases)}] {case['case_id']} {len(case['turns'])} turns", flush=True)
        try:
            results.append(run_case(args.base_url, case, args.request_timeout))
        except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, OSError, RuntimeError) as error:
            results.append({
                "case_id": case.get("case_id", f"case-{index}"),
                "passed": False,
                "failures": [str(error)],
                "latency_ms": 0,
                "total_tokens": 0,
            })
    passed = sum(1 for item in results if item.get("passed"))
    tool_ok = sum(1 for item in results if item.get("tool_accuracy", False))
    false_actions = sum(1 for item in results if item.get("false_action", False))
    pii_leaks = sum(1 for item in results if item.get("pii_leak", False))
    latencies = [int(item.get("latency_ms") or 0) for item in results]
    tokens = [int(item.get("total_tokens") or 0) for item in results]
    report = {
        "run_id": "multiturn_" + dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d_%H%M%S"),
        "generated_at": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC"),
        "base_url": args.base_url,
        "total_cases": len(results),
        "passed_cases": passed,
        "pass_rate": passed / len(results) if results else 0,
        "tool_accuracy": tool_ok / len(results) if results else 0,
        "false_action_rate": false_actions / len(results) if results else 0,
        "pii_leak_rate": pii_leaks / len(results) if results else 0,
        "average_tokens": sum(tokens) / len(tokens) if tokens else 0,
        "p95_latency_ms": percentile95(latencies),
        "results": results,
    }
    if args.json_output:
        os.makedirs(os.path.dirname(args.json_output) or ".", exist_ok=True)
        with open(args.json_output, "w", encoding="utf-8") as target:
            json.dump(report, target, ensure_ascii=False, indent=2)
    write_markdown(args.output, report)
    print(json.dumps({
        "total_cases": report["total_cases"],
        "passed_cases": report["passed_cases"],
        "pass_rate": report["pass_rate"],
        "tool_accuracy": report["tool_accuracy"],
        "false_action_rate": report["false_action_rate"],
        "pii_leak_rate": report["pii_leak_rate"],
        "average_tokens": report["average_tokens"],
        "p95_latency_ms": report["p95_latency_ms"],
        "output": args.output,
        "json_output": args.json_output,
    }, ensure_ascii=False, indent=2))
    if passed != len(results):
        sys.exit(1)


if __name__ == "__main__":
    main()

