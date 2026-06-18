#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import os
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
    with urllib.request.urlopen(request, timeout=timeout) as response:
        body = response.read().decode("utf-8")
        return response.status, json.loads(body)


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


def collect_plan_context(trace_data):
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
    return " ".join(queries), params


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
    _, trace_envelope = request_json(
        "GET",
        base_url,
        f"/api/v1/admin/traces/{trace_id}",
        timeout=request_timeout,
    )
    trace_data = envelope_data(trace_envelope)
    plan_query, plan_params = collect_plan_context(trace_data)
    answer = final_response.get("answer") or ""
    input_tokens = int(trace_data.get("input_tokens") or 0)
    output_tokens = int(trace_data.get("output_tokens") or 0)
    haystack = "\n".join([plan_query, answer])

    failures = []
    expected_intent = case.get("expected_intent")
    actual_intent = trace_data.get("intent")
    if expected_intent and actual_intent != expected_intent:
        failures.append(f"intent {actual_intent!r} != {expected_intent!r}")
    for key, expected in (case.get("expected_entities") or {}).items():
        actual = str(plan_params.get(key, ""))
        if actual != str(expected):
            failures.append(f"entity {key} {actual!r} != {expected!r}")
    for term in case.get("required_query_terms") or []:
        if term not in plan_query:
            failures.append(f"plan query missing {term!r}")
    for term in case.get("required_answer_terms") or []:
        if term not in answer:
            failures.append(f"answer missing {term!r}")
    for phrase in case.get("forbidden_answer_terms") or []:
        if phrase in haystack:
            failures.append(f"forbidden phrase present {phrase!r}")
    if case.get("require_tokens", True) and input_tokens + output_tokens <= 0:
        failures.append("trace has no token usage")

    return {
        "case_id": case["case_id"],
        "passed": not failures,
        "failures": failures,
        "conversation_id": conversation_id,
        "final_trace_id": trace_id,
        "actual_intent": actual_intent,
        "plan_query": plan_query,
        "plan_params": plan_params,
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
    latencies = [int(item.get("latency_ms") or 0) for item in results]
    tokens = [int(item.get("total_tokens") or 0) for item in results]
    report = {
        "run_id": "multiturn_" + dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d_%H%M%S"),
        "generated_at": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC"),
        "base_url": args.base_url,
        "total_cases": len(results),
        "passed_cases": passed,
        "pass_rate": passed / len(results) if results else 0,
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
        "average_tokens": report["average_tokens"],
        "p95_latency_ms": report["p95_latency_ms"],
        "output": args.output,
        "json_output": args.json_output,
    }, ensure_ascii=False, indent=2))
    if passed != len(results):
        sys.exit(1)


if __name__ == "__main__":
    main()
