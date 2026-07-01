import fs from 'node:fs/promises';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');
const require = createRequire(path.join(repoRoot, 'clean-care-frontend', 'package.json'));
const { chromium } = require('playwright');
const casesPath = path.resolve(
  repoRoot,
  process.env.CASES_PATH || 'docs/eval/frontend-agentic-rag-50.json'
);
const frontendURL = process.env.FRONTEND_URL || 'http://127.0.0.1:5173';
const outputDir = process.env.REPORT_DIR || 'D:\\Codex\\outputs';
const maxCases = Number.parseInt(process.env.MAX_CASES || '50', 10);
const caseTimeoutMs = Number.parseInt(process.env.CASE_TIMEOUT_MS || '90000', 10);
const headless = (process.env.HEADLESS || 'true').toLowerCase() !== 'false';
const runUISmoke = (process.env.UI_SMOKE || 'true').toLowerCase() !== 'false';
const llmJudgeEnabled = (process.env.LLM_JUDGE || 'false').toLowerCase() === 'true';
const judgeEndpoint = process.env.JUDGE_ENDPOINT || process.env.CLEANCARE_LLM_ENDPOINT || '';
const judgeAPIKey = process.env.JUDGE_API_KEY || process.env.CLEANCARE_LLM_API_KEY || '';
const judgeModel = process.env.JUDGE_MODEL || process.env.CLEANCARE_LLM_MODEL || '';

const raw = await fs.readFile(casesPath, 'utf8');
const dataset = JSON.parse(raw);
const cases = dataset.cases.slice(0, maxCases);
await fs.mkdir(outputDir, { recursive: true });

const browser = await chromium.launch({ headless });
const page = await browser.newPage({ viewport: { width: 1440, height: 960 } });

const results = [];
const startedAt = new Date();

try {
  await page.goto(`${frontendURL.replace(/\/$/, '')}/chat`, { waitUntil: 'networkidle' });
  await page.waitForSelector('textarea.chat-input__textarea', { timeout: 30000 });

  let startIndex = 0;
  if (runUISmoke && cases.length > 0) {
    const first = cases[0];
    const uiResult = await runUIMessage(page, first, caseTimeoutMs);
    results.push(uiResult);
    startIndex = 1;
  }

  for (let index = startIndex; index < cases.length; index++) {
    const item = cases[index];
    const result = await runStreamCase(page, item, caseTimeoutMs);
    results.push(result);
    const status = result.ok ? 'ok' : 'fail';
    console.log(`${index + 1}/${cases.length} ${status} ${item.id} ${result.duration_ms}ms`);
  }
} finally {
  await browser.close();
}

await enrichSemanticJudgements(results, cases);
const report = buildReport(dataset.dataset_version, startedAt, new Date(), results);
const timestamp = startedAt.toISOString().replace(/[:.]/g, '-');
const jsonPath = path.join(outputDir, `frontend-agentic-rag-report-${timestamp}.json`);
const mdPath = path.join(outputDir, `frontend-agentic-rag-report-${timestamp}.md`);
await fs.writeFile(jsonPath, JSON.stringify(report, null, 2), 'utf8');
await fs.writeFile(mdPath, renderMarkdown(report), 'utf8');

console.log(`report_json=${jsonPath}`);
console.log(`report_md=${mdPath}`);
if (report.summary.failed > 0 || report.summary.semantic_failed > 0) {
  process.exitCode = 1;
}

async function runUIMessage(page, item, timeoutMs) {
  const started = Date.now();
  try {
    const before = await page.locator('.message--assistant').count();
    await page.fill('textarea.chat-input__textarea', item.turns[0]);
    await page.keyboard.press('Enter');
    await page.waitForFunction(
      (count) => {
        const nodes = Array.from(document.querySelectorAll('.message--assistant .message__body'));
        return nodes.length > count && nodes[nodes.length - 1].textContent.trim().length > 0;
      },
      before,
      { timeout: timeoutMs }
    );
    const answer = await page
      .locator('.message--assistant .message__body')
      .last()
      .innerText({ timeout: 5000 });
    return finishCase(item, Date.now() - started, answer, {
      ok: true,
      transport: 'react-ui-sse',
      turns: [{ content: item.turns[0], answer }],
    });
  } catch (error) {
    return finishCase(item, Date.now() - started, '', {
      ok: false,
      transport: 'react-ui-sse',
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

async function runStreamCase(page, item, timeoutMs) {
  return page.evaluate(
    async ({ item, timeoutMs }) => {
      const started = performance.now();
      const headers = {
        Authorization: 'Bearer mock-jwt-demo-token',
        'Content-Type': 'application/json',
      };
      const result = {
        id: item.id,
        product: item.product,
        intent: item.intent,
        ok: true,
        transport: 'browser-fetch-sse',
        duration_ms: 0,
        turns: [],
        trace_ids: [],
        mode: '',
        status_events: 0,
        evidence_events: 0,
        tool_names: [],
        error: '',
      };
      try {
        const createResponse = await fetch('/api/v1/conversations', {
          method: 'POST',
          headers,
          body: JSON.stringify({ title: item.id }),
        });
        if (!createResponse.ok) {
          throw new Error(`create conversation HTTP ${createResponse.status}`);
        }
        const envelope = await createResponse.json();
        const conversationID = envelope.data?.conversation_id;
        if (!conversationID) {
          throw new Error('missing conversation_id');
        }

        for (const content of item.turns) {
          const controller = new AbortController();
          const timeout = setTimeout(() => controller.abort(), timeoutMs);
          const turnStarted = performance.now();
          let answer = '';
          let traceID = '';
          let mode = '';
          try {
            const response = await fetch(
              `/api/v1/conversations/${conversationID}/messages:stream`,
              {
                method: 'POST',
                headers: { ...headers, Accept: 'text/event-stream' },
                body: JSON.stringify({ content }),
                signal: controller.signal,
              }
            );
            if (!response.ok) {
              throw new Error(`stream HTTP ${response.status}`);
            }
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';
            let completed = false;
            const consumeFrame = (frame) => {
              if (!frame.trim()) return false;
              const event = parseSSEFrame(frame);
              if (event.type === 'status') {
                result.status_events += 1;
                if (event.data?.trace_id) traceID = event.data.trace_id;
              } else if (event.type === 'evidence') {
                result.evidence_events += 1;
              } else if (event.type === 'delta') {
                answer += event.data?.content || '';
              } else if (event.type === 'done') {
                traceID = event.data?.trace_id || traceID;
                mode = event.data?.mode || mode;
                return true;
              } else if (event.type === 'error') {
                throw new Error(event.data?.message || event.data?.code || 'stream error');
              }
              return false;
            };
            while (true) {
              const { done, value } = await reader.read();
              if (done) {
                buffer += decoder.decode();
                break;
              }
              buffer += decoder.decode(value, { stream: true });
              const frames = buffer.split('\n\n');
              buffer = frames.pop() || '';
              for (const frame of frames) {
                if (consumeFrame(frame)) {
                  completed = true;
                  break;
                }
              }
              if (completed) {
                await reader.cancel().catch(() => {});
                break;
              }
            }
            if (!completed && buffer.trim()) {
              const frames = buffer.split('\n\n');
              for (const frame of frames) {
                if (consumeFrame(frame)) {
                  completed = true;
                  break;
                }
              }
            }
          } finally {
            clearTimeout(timeout);
          }
          result.turns.push({
            content,
            answer,
            duration_ms: Math.round(performance.now() - turnStarted),
            trace_id: traceID,
            answer_length: answer.length,
          });
          if (traceID) {
            result.trace_ids.push(traceID);
            const tools = await fetchTraceTools(traceID);
            for (const name of tools) {
              if (!result.tool_names.includes(name)) result.tool_names.push(name);
            }
          }
          if (mode) result.mode = mode;
          if (!answer.trim()) {
            throw new Error(`empty answer for turn: ${content}`);
          }
        }
      } catch (error) {
        result.ok = false;
        result.error = error instanceof Error ? error.message : String(error);
      }
      result.duration_ms = Math.round(performance.now() - started);
      return result;

      function parseSSEFrame(frame) {
        let type = '';
        const dataLines = [];
        for (const line of frame.split('\n')) {
          if (line.startsWith('event: ')) {
            type = line.slice(7).trim();
          } else if (line.startsWith('data: ')) {
            dataLines.push(line.slice(6));
          }
        }
        let data = {};
        const raw = dataLines.join('\n');
        if (raw) {
          try {
            data = JSON.parse(raw);
          } catch {
            data = { raw };
          }
        }
        return { type, data };
      }

      async function fetchTraceTools(traceID) {
        try {
          const response = await fetch(`/api/v1/admin/traces/${encodeURIComponent(traceID)}`, {
            headers: { Authorization: 'Bearer mock-jwt-demo-token' },
          });
          if (!response.ok) return [];
          const envelope = await response.json();
          const calls = envelope.data?.tool_calls || [];
          return calls.map((call) => call.tool_name).filter(Boolean);
        } catch {
          return [];
        }
      }
    },
    { item, timeoutMs }
  ).then((result) => finishCase(item, result.duration_ms, lastAnswer(result), result));
}

function finishCase(item, durationMs, answer, result) {
  const expected = item.expected_signals || [];
  const answerText = answer || '';
  const signalText = [
    answerText,
    ...(result.trace_ids || []),
    ...(result.tool_names || []),
  ].join('\n');
  const hits = expected.filter((signal) => signalMatched(signal, signalText));
  const signalOK = expected.length === 0 || hits.length === expected.length;
  return {
    id: item.id,
    product: item.product,
    intent: item.intent,
    ok: Boolean(result.ok),
    signal_ok: signalOK,
    transport: result.transport,
    duration_ms: Math.round(durationMs),
    mode: result.mode || '',
    trace_ids: result.trace_ids || [],
    tool_names: result.tool_names || [],
    status_events: result.status_events || 0,
    evidence_events: result.evidence_events || 0,
    expected_hits: hits,
    expected_total: expected.length,
    gold_ok: null,
    semantic_ok: null,
    llm_judge: null,
    answer_length: answerText.length,
    turns: result.turns || [],
    error: result.error || '',
  };
}

async function enrichSemanticJudgements(results, cases) {
  const casesByID = new Map(cases.map((item) => [item.id, item]));
  const canUseJudge = llmJudgeEnabled && judgeEndpoint && judgeAPIKey && judgeModel;
  for (const result of results) {
    const item = casesByID.get(result.id);
    if (!item) continue;
    const answer = lastAnswer(result);
    const goldMustInclude = item.gold_must_include || item.must_include || [];
    if (goldMustInclude.length > 0) {
      result.gold_ok = goldMustInclude.every((signal) => signalMatched(signal, answer));
      result.semantic_ok = result.gold_ok;
    }
    const goldAnswer = item.gold_answer || item.expected_answer || '';
    if (canUseJudge && goldAnswer) {
      result.llm_judge = await runLLMJudge(item, answer, goldAnswer);
      result.semantic_ok = Boolean(result.llm_judge.pass);
    }
  }
}

async function runLLMJudge(item, answer, goldAnswer) {
  try {
    const endpoint = chatCompletionsURL(judgeEndpoint);
    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${judgeAPIKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: judgeModel,
        temperature: 0,
        max_tokens: 200,
        messages: [
          {
            role: 'system',
            content: '你是严谨的客服 RAG 评测裁判。只输出 JSON：{"score":0-1,"pass":true|false,"reason":"简短原因"}。',
          },
          {
            role: 'user',
            content: [
              `问题：${item.turns.join('\n')}`,
              `标准答案要点：${goldAnswer}`,
              `候选答案：${answer}`,
              '判定候选答案是否覆盖标准答案要点，且没有明显无依据或相反结论。pass 阈值为 0.75。',
            ].join('\n\n'),
          },
        ],
      }),
    });
    if (!response.ok) {
      return { score: 0, pass: false, reason: `judge HTTP ${response.status}` };
    }
    const payload = await response.json();
    const content = payload.choices?.[0]?.message?.content || '';
    const parsed = parseJudgeJSON(content);
    return {
      score: Number(parsed.score || 0),
      pass: Boolean(parsed.pass),
      reason: String(parsed.reason || '').slice(0, 300),
    };
  } catch (error) {
    return {
      score: 0,
      pass: false,
      reason: error instanceof Error ? error.message : String(error),
    };
  }
}

function chatCompletionsURL(endpoint) {
  const trimmed = endpoint.replace(/\/$/, '');
  if (trimmed.endsWith('/chat/completions')) return trimmed;
  return `${trimmed}/chat/completions`;
}

function parseJudgeJSON(content) {
  try {
    return JSON.parse(content);
  } catch {
    const match = content.match(/\{[\s\S]*\}/);
    if (!match) return {};
    try {
      return JSON.parse(match[0]);
    } catch {
      return {};
    }
  }
}

function signalMatched(signal, text) {
  const value = String(signal);
  if (
    value === '兼容' &&
    /(无法判断|不能判断|不确定|无法确认|不能确认|暂未收录[^。；\n]*兼容|兼容[^。；\n]*暂未收录)/.test(text)
  ) {
    return false;
  }
  if (value === '不适用') {
    return /(不适用|不建议|不适合|不能用于|不应用于)/.test(text);
  }
  return text.toLowerCase().includes(value.toLowerCase());
}

function lastAnswer(result) {
  if (!result.turns || result.turns.length === 0) return '';
  return result.turns[result.turns.length - 1].answer || '';
}

function buildReport(version, started, ended, results) {
  const durations = results.filter((r) => r.ok).map((r) => r.duration_ms).sort((a, b) => a - b);
  const failed = results.filter((r) => !r.ok);
  const signalFailed = results.filter((r) => !r.signal_ok);
  const semanticResults = results.filter((r) => r.semantic_ok !== null);
  const semanticFailed = semanticResults.filter((r) => !r.semantic_ok);
  const expectedTotal = results.reduce((sum, result) => sum + result.expected_total, 0);
  const expectedHits = results.reduce((sum, result) => sum + result.expected_hits.length, 0);
  const byIntent = {};
  for (const result of results) {
    byIntent[result.intent] ||= { total: 0, passed: 0, signal_passed: 0, semantic_total: 0, semantic_passed: 0 };
    byIntent[result.intent].total += 1;
    if (result.ok) byIntent[result.intent].passed += 1;
    if (result.signal_ok) byIntent[result.intent].signal_passed += 1;
    if (result.semantic_ok !== null) {
      byIntent[result.intent].semantic_total += 1;
      if (result.semantic_ok) byIntent[result.intent].semantic_passed += 1;
    }
  }
  return {
    dataset_version: version,
    started_at: started.toISOString(),
    ended_at: ended.toISOString(),
    summary: {
      total: results.length,
      passed: results.length - failed.length,
      failed: failed.length,
      success_rate: results.length === 0 ? 0 : Number(((results.length - failed.length) / results.length).toFixed(4)),
      signal_passed: results.length - signalFailed.length,
      signal_failed: signalFailed.length,
      signal_success_rate: results.length === 0 ? 0 : Number(((results.length - signalFailed.length) / results.length).toFixed(4)),
      expected_signal_hits: expectedHits,
      expected_signal_total: expectedTotal,
      semantic_total: semanticResults.length,
      semantic_passed: semanticResults.length - semanticFailed.length,
      semantic_failed: semanticFailed.length,
      semantic_success_rate: semanticResults.length === 0 ? 0 : Number(((semanticResults.length - semanticFailed.length) / semanticResults.length).toFixed(4)),
      p50_ms: percentile(durations, 0.5),
      p90_ms: percentile(durations, 0.9),
      p95_ms: percentile(durations, 0.95),
      max_ms: durations[durations.length - 1] || 0,
    },
    by_intent: byIntent,
    failures: failed.map((r) => ({ id: r.id, product: r.product, intent: r.intent, error: r.error })),
    signal_failures: signalFailed.map((r) => ({
      id: r.id,
      product: r.product,
      intent: r.intent,
      expected_hits: r.expected_hits,
      expected_total: r.expected_total,
      tool_names: r.tool_names,
    })),
    semantic_failures: semanticFailed.map((r) => ({
      id: r.id,
      product: r.product,
      intent: r.intent,
      gold_ok: r.gold_ok,
      llm_judge: r.llm_judge,
    })),
    results,
  };
}

function percentile(values, p) {
  if (values.length === 0) return 0;
  const index = Math.min(values.length - 1, Math.ceil(values.length * p) - 1);
  return values[index];
}

function renderMarkdown(report) {
  const lines = [];
  lines.push(`# Frontend Agentic RAG Report`);
  lines.push('');
  lines.push(`Dataset: \`${report.dataset_version}\``);
  lines.push(`Started: \`${report.started_at}\``);
  lines.push(`Ended: \`${report.ended_at}\``);
  lines.push('');
  lines.push(`- Total: ${report.summary.total}`);
  lines.push(`- Passed: ${report.summary.passed}`);
  lines.push(`- Failed: ${report.summary.failed}`);
  lines.push(`- Success rate: ${(report.summary.success_rate * 100).toFixed(2)}%`);
  lines.push(`- Signal passed: ${report.summary.signal_passed}`);
  lines.push(`- Signal failed: ${report.summary.signal_failed}`);
  lines.push(`- Signal success rate: ${(report.summary.signal_success_rate * 100).toFixed(2)}%`);
  lines.push(`- Expected signal hits: ${report.summary.expected_signal_hits}/${report.summary.expected_signal_total}`);
  if (report.summary.semantic_total > 0) {
    lines.push(`- Semantic passed: ${report.summary.semantic_passed}/${report.summary.semantic_total}`);
    lines.push(`- Semantic success rate: ${(report.summary.semantic_success_rate * 100).toFixed(2)}%`);
  }
  lines.push(`- p50: ${report.summary.p50_ms} ms`);
  lines.push(`- p90: ${report.summary.p90_ms} ms`);
  lines.push(`- p95: ${report.summary.p95_ms} ms`);
  lines.push(`- Max: ${report.summary.max_ms} ms`);
  lines.push('');
  lines.push(`## By Intent`);
  for (const [intent, item] of Object.entries(report.by_intent)) {
    const semantic = item.semantic_total > 0 ? `, semantic ${item.semantic_passed}/${item.semantic_total}` : '';
    lines.push(`- ${intent}: chain ${item.passed}/${item.total}, signals ${item.signal_passed}/${item.total}${semantic}`);
  }
  if (report.failures.length > 0) {
    lines.push('');
    lines.push(`## Failures`);
    for (const failure of report.failures) {
      lines.push(`- ${failure.id} (${failure.product}, ${failure.intent}): ${failure.error}`);
    }
  }
  if (report.signal_failures.length > 0) {
    lines.push('');
    lines.push(`## Signal Misses`);
    for (const failure of report.signal_failures) {
      lines.push(`- ${failure.id} (${failure.product}, ${failure.intent}): ${failure.expected_hits.length}/${failure.expected_total} expected signals, tools=${(failure.tool_names || []).join(',')}`);
    }
  }
  if (report.semantic_failures.length > 0) {
    lines.push('');
    lines.push(`## Semantic Misses`);
    for (const failure of report.semantic_failures) {
      const judge = failure.llm_judge ? `, judge=${failure.llm_judge.score}: ${failure.llm_judge.reason}` : '';
      lines.push(`- ${failure.id} (${failure.product}, ${failure.intent}): gold_ok=${failure.gold_ok}${judge}`);
    }
  }
  return lines.join('\n');
}
