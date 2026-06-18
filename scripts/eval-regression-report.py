#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import re


def request_json(method, base_url, path, payload=None, timeout=60):
    data = None
    headers = {"Content-Type": "application/json"}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
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


def wait_for_run(base_url, run_no, poll_interval, timeout_seconds, request_timeout):
    deadline = time.monotonic() + timeout_seconds
    last = None
    last_error = None
    while time.monotonic() < deadline:
        try:
            _, envelope = request_json(
                "GET",
                base_url,
                f"/api/v1/admin/eval/runs/{run_no}?include_failures=false",
                timeout=request_timeout,
            )
            last_error = None
        except (TimeoutError, urllib.error.URLError, ConnectionError, socket.timeout, OSError, RuntimeError) as error:
            last_error = error
            time.sleep(poll_interval)
            continue
        if envelope.get("code") != "OK":
            raise RuntimeError(f"eval query failed: {envelope}")
        last = envelope["data"]
        if last.get("status") in {"completed", "failed"}:
            return last
        time.sleep(poll_interval)
    raise TimeoutError(f"timed out waiting for eval run {run_no}; last={last}; last_error={last_error}")


def fetch_failures(base_url, run_no, request_timeout, retries):
    last_error = None
    for attempt in range(max(1, retries)):
        try:
            _, envelope = request_json(
                "GET",
                base_url,
                f"/api/v1/admin/eval/runs/{run_no}?include_failures=true",
                timeout=request_timeout,
            )
            if envelope.get("code") != "OK":
                raise RuntimeError(f"eval query failed: {envelope}")
            return envelope["data"]
        except (TimeoutError, urllib.error.URLError, ConnectionError, socket.timeout, OSError, RuntimeError) as error:
            last_error = error
            if attempt + 1 < max(1, retries):
                time.sleep(2)
    print(f"warning: failed to fetch eval failure details for {run_no}: {last_error}", file=sys.stderr)
    return None


def number(value, default=0):
    if isinstance(value, (int, float)):
        return value
    return default


def percent(value):
    if value is None:
        return "n/a"
    return f"{float(value) * 100:.1f}%"


def delta(current, baseline, suffix=""):
    if current is None or baseline is None:
        return "n/a"
    value = float(current) - float(baseline)
    sign = "+" if value >= 0 else ""
    return f"{sign}{value:.1f}{suffix}"


def percent_point_delta(current, baseline):
    if current is None or baseline is None:
        return "n/a"
    value = (float(current) - float(baseline)) * 100
    sign = "+" if value >= 0 else ""
    return f"{sign}{value:.1f} pp"


def ensure_parent(path):
    parent = os.path.dirname(path)
    if parent:
        os.makedirs(parent, exist_ok=True)


def render_kv_table(values, value_label="Count"):
    if not values:
        return "| Type | " + value_label + " |\n|---|---:|\n| none | 0 |\n"
    rows = ["| Type | " + value_label + " |", "|---|---:|"]
    for key, value in sorted(values.items(), key=lambda item: (-number(item[1]), str(item[0]))):
        rows.append(f"| `{key or 'unknown'}` | {value} |")
    return "\n".join(rows) + "\n"


def render_group_table(title, groups):
    if not groups:
        return f"## {title}\n\nNo group data recorded.\n"
    rows = [f"## {title}", "", "| Group | Passed | Total | Pass Rate |", "|---|---:|---:|---:|"]
    for key, value in sorted(groups.items()):
        total = int(number(value.get("total") if isinstance(value, dict) else None))
        passed = int(number(value.get("passed") if isinstance(value, dict) else None))
        pass_rate = value.get("pass_rate") if isinstance(value, dict) else None
        rows.append(f"| `{key}` | {passed} | {total} | {percent(pass_rate)} |")
    rows.append("")
    return "\n".join(rows)


def render_failure_rows(results, limit):
    if not results:
        return "No failed cases were returned by the API.\n"
    rows = ["| Case | Error Type | Intent | Tools | Latency | Trace |", "|---|---|---|---|---:|---|"]
    for item in results[:limit]:
        tools = ", ".join(item.get("actual_tools") or [])
        rows.append(
            "| `{case}` | `{error}` | `{intent}` | {tools} | {latency} ms | `{trace}` |".format(
                case=item.get("case_id", ""),
                error=item.get("error_type") or "unknown",
                intent=item.get("actual_intent") or "",
                tools=tools or "-",
                latency=item.get("latency_ms", 0),
                trace=item.get("trace_id") or "",
            )
        )
    if len(results) > limit:
        rows.append(f"| ... | {len(results) - limit} more failures omitted |  |  |  |  |")
    return "\n".join(rows) + "\n"


def write_report(path, run, args):
    summary = run.get("summary") or {}
    total_cases = int(number(summary.get("total_cases")))
    passed_cases = int(number(summary.get("passed_cases")))
    pass_rate = summary.get("pass_rate")
    p95_latency = summary.get("p95_latency_ms")
    average_tokens = summary.get("average_tokens")
    average_steps = summary.get("average_react_steps")
    metrics = summary.get("metrics") or {}
    failure_types = summary.get("failure_types") or {}
    generated_at = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")

    lines = [
        "# MCP HTTP 回归评测记录",
        "",
        f"生成时间：{generated_at}",
        f"运行编号：`{run.get('run_no', '')}`",
        f"系统版本：`{run.get('system_version', '')}`",
        f"数据集：`{run.get('dataset_version', '')}`",
        f"状态：`{run.get('status', '')}`",
        "",
        "## 摘要",
        "",
        "| 指标 | 结果 |",
        "|---|---:|",
        f"| Total Cases | {total_cases} |",
        f"| Passed Cases | {passed_cases} |",
        f"| Strict Pass Rate | {percent(pass_rate)} |",
        f"| P95 Latency | {int(number(p95_latency))} ms |",
        f"| Average Tokens | {number(average_tokens):.2f} |",
        f"| Average ReAct Steps | {number(average_steps):.2f} |",
        "",
        "## 变化参考",
        "",
    ]
    if args.baseline_label:
        baseline_pass = args.baseline_pass_rate
        baseline_p95 = args.baseline_p95_latency_ms
        baseline_tokens = args.baseline_average_tokens
        lines.extend([
            "参考基线来自历史报告，若模型、Embedding、Reranker 或数据集不同，只能用于观察趋势，不能作为严格 A/B 结论。",
            "",
            "| Reference | Pass Rate | Pass Delta | P95 Latency | P95 Delta | Avg Tokens | Token Delta |",
            "|---|---:|---:|---:|---:|---:|---:|",
            (
                f"| {args.baseline_label} | {percent(baseline_pass)} | {percent_point_delta(pass_rate, baseline_pass)} | "
                f"{int(number(baseline_p95))} ms | {delta(p95_latency, baseline_p95, ' ms')} | "
                f"{number(baseline_tokens):.2f} | {delta(average_tokens, baseline_tokens)} |"
            ),
            "",
        ])
    else:
        lines.extend([
            "未提供可比较基线。本文件记录 MCP HTTP 版本后的当前回归基线；后续运行应使用相同数据集和模型栈对比。",
            "",
        ])
    lines.extend([
        "## 失败类型",
        "",
        render_kv_table(failure_types).rstrip(),
        "",
        "## 核心指标",
        "",
        "| Metric | Value |",
        "|---|---:|",
    ])
    for name, value in sorted(metrics.items()):
        lines.append(f"| `{name}` | {number(value):.4f} |")
    lines.extend([
        "",
        render_group_table("按意图", summary.get("by_intent") or {}).rstrip(),
        "",
        render_group_table("按难度", summary.get("by_difficulty") or {}).rstrip(),
        "",
        render_group_table("按路径", summary.get("by_path") or {}).rstrip(),
        "",
        "## 失败样例",
        "",
        render_failure_rows(run.get("results") or [], args.failure_limit).rstrip(),
        "",
        "## 复现",
        "",
        "```powershell",
        (
            "python3 scripts/eval-regression-report.py "
            f"--base-url {args.base_url} "
            f"--system-version {run.get('system_version', '')} "
            f"--max-cases {args.max_cases} "
            f"--output {path}"
        ),
        "```",
        "",
    ])
    ensure_parent(path)
    with open(path, "w", encoding="utf-8") as report:
        report.write("\n".join(lines))


def parse_args():
    today = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d")
    def env_float(name):
        raw = os.environ.get(name)
        return float(raw) if raw not in (None, "") else None

    parser = argparse.ArgumentParser(description="Run eval regression and write a Markdown report.")
    parser.add_argument("--base-url", default=os.environ.get("BASE_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--dataset-version", default=os.environ.get("EVAL_DATASET_VERSION", "v2"))
    parser.add_argument(
        "--system-version",
        default=os.environ.get("SYSTEM_VERSION", f"agentic-mcp-http-{today}"),
    )
    parser.add_argument("--run-no", default=os.environ.get("EVAL_RUN_NO", ""))
    parser.add_argument("--max-cases", type=int, default=int(os.environ.get("EVAL_MAX_CASES", "200")))
    parser.add_argument("--case-id", action="append", default=[])
    parser.add_argument(
        "--case-id-range",
        action="append",
        default=[],
        help="Inclusive case id range such as EVAL-101:EVAL-200. Can be repeated.",
    )
    parser.add_argument("--output", default=os.environ.get("EVAL_OUTPUT", "docs/eval/mcp-regression-report.md"))
    parser.add_argument("--json-output", default=os.environ.get("EVAL_JSON_OUTPUT", ""))
    parser.add_argument("--poll-interval", type=float, default=float(os.environ.get("EVAL_POLL_INTERVAL", "5")))
    parser.add_argument("--timeout-seconds", type=int, default=int(os.environ.get("EVAL_TIMEOUT_SECONDS", "7200")))
    parser.add_argument("--poll-request-timeout", type=float, default=float(os.environ.get("EVAL_POLL_REQUEST_TIMEOUT", "30")))
    parser.add_argument("--result-request-timeout", type=float, default=float(os.environ.get("EVAL_RESULT_REQUEST_TIMEOUT", "180")))
    parser.add_argument("--result-retries", type=int, default=int(os.environ.get("EVAL_RESULT_RETRIES", "2")))
    parser.add_argument("--failure-limit", type=int, default=int(os.environ.get("EVAL_FAILURE_LIMIT", "50")))
    parser.add_argument("--baseline-label", default=os.environ.get("EVAL_BASELINE_LABEL", ""))
    parser.add_argument("--baseline-pass-rate", type=float, default=env_float("EVAL_BASELINE_PASS_RATE"))
    parser.add_argument("--baseline-p95-latency-ms", type=float, default=env_float("EVAL_BASELINE_P95_LATENCY_MS"))
    parser.add_argument("--baseline-average-tokens", type=float, default=env_float("EVAL_BASELINE_AVERAGE_TOKENS"))
    return parser.parse_args()


def expand_case_id_ranges(ranges):
    case_ids = []
    pattern = re.compile(r"^([A-Za-z_-]*?)(\d+):([A-Za-z_-]*?)(\d+)$")
    for spec in ranges:
        spec = spec.strip()
        if not spec:
            continue
        match = pattern.match(spec)
        if not match:
            raise ValueError(f"invalid --case-id-range {spec!r}; expected EVAL-101:EVAL-200")
        start_prefix, start_num, end_prefix, end_num = match.groups()
        if end_prefix and end_prefix != start_prefix:
            raise ValueError(f"range prefixes differ in {spec!r}")
        width = max(len(start_num), len(end_num))
        start = int(start_num)
        end = int(end_num)
        if end < start:
            raise ValueError(f"range end precedes start in {spec!r}")
        for value in range(start, end + 1):
            case_ids.append(f"{start_prefix}{value:0{width}d}")
    return case_ids


def main():
    args = parse_args()
    args.case_id.extend(expand_case_id_ranges(args.case_id_range))
    if args.run_no:
        run_no = args.run_no
    else:
        payload = {
            "dataset_version": args.dataset_version,
            "system_version": args.system_version,
            "max_cases": args.max_cases,
        }
        if args.case_id:
            payload["case_ids"] = args.case_id
        status, envelope = request_json("POST", args.base_url, "/api/v1/admin/eval/runs", payload, timeout=60)
        if status != 202 or envelope.get("code") != "OK":
            raise RuntimeError(f"eval start failed: {envelope}")
        run_no = envelope["data"]["run_no"]
    run = wait_for_run(args.base_url, run_no, args.poll_interval, args.timeout_seconds, args.poll_request_timeout)
    if run.get("status") in {"completed", "failed"}:
        detailed_run = fetch_failures(args.base_url, run_no, args.result_request_timeout, args.result_retries)
        if detailed_run is not None:
            run = detailed_run
    if args.json_output:
        ensure_parent(args.json_output)
        with open(args.json_output, "w", encoding="utf-8") as raw:
            json.dump(run, raw, ensure_ascii=False, indent=2)
    write_report(args.output, run, args)
    print(json.dumps({
        "run_no": run_no,
        "status": run.get("status"),
        "summary": run.get("summary"),
        "output": args.output,
        "json_output": args.json_output,
    }, ensure_ascii=False, indent=2))
    if run.get("status") != "completed":
        raise RuntimeError(f"eval run {run_no} ended with status {run.get('status')}")


if __name__ == "__main__":
    main()
