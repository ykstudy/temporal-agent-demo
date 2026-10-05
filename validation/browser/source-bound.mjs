import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';

// Public static-UI consumer. No product module, runtime API, or generator imports.
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const browserURL = process.env.AGENT_BROWSER_URL || 'http://127.0.0.1:15181/';
const reportDir = path.resolve(process.env.AGENT_BROWSER_REPORT_DIR || '/tmp/agent-vue-review-qa/loader-final');
const digest = text => crypto.createHash('sha256').update(text, 'utf8').digest('hex');
const sourceAt = (bundle, sourcePath) => bundle.sources.find(source => source.path === sourcePath);
const productPath = 'docs/PRODUCT-GUIDE.md';
const invariantPath = 'design/00-invariants/INVARIANTS.md';
const schemaPath = 'design/review/review-bundle.schema.json';
const publicationMismatch = '审查发布摘要不一致';
const cases = [
  { id: 'baseline_product_false', mutate: bundle => { bundle.baseline.source_sha256 = '0'.repeat(64); } },
  { id: 'baseline_invariants_false', mutate: bundle => { bundle.baseline.invariants_sha256 = '0'.repeat(64); } },
  { id: 'network_schema_relaxed_with_extra', mutate: (bundle, schema) => { schema.additionalProperties = true; bundle.unexpected_for_review = 'must_reject'; } },
  { id: 'extra_field_original_schema', mutate: bundle => { bundle.unexpected_for_review = 'must_reject'; } },
  { id: 'schema_source_text_resigned', mutate: bundle => { const source = sourceAt(bundle, schemaPath); source.text += '\n'; source.sha256 = digest(source.text); } },
  { id: 'schema_source_missing', mutate: bundle => { bundle.sources = bundle.sources.filter(source => source.path !== schemaPath); } },
  { id: 'product_source_missing', mutate: bundle => { bundle.sources = bundle.sources.filter(source => source.path !== productPath); } },
  { id: 'invariants_source_resigned', mutate: bundle => { const source = sourceAt(bundle, invariantPath); source.text += '\nchanged'; source.sha256 = digest(source.text); bundle.baseline.invariants_sha256 = source.sha256; } },
  { id: 'source_text_digest_mismatch', mutate: bundle => { bundle.sources[0].text += '\nchanged'; } },
  { id: 'source_duplicate_path', mutate: bundle => { bundle.sources.push({ ...bundle.sources[0], id: 'source.duplicate-path' }); } },
  { id: 'source_duplicate_id', mutate: bundle => { bundle.sources.push({ ...bundle.sources[0] }); } },
  { id: 'source_dangling_binding', mutate: bundle => { bundle.models[0].source_ids[0] = 'source.not-published'; } },
  { id: 'coverage_missing_cell', mutate: bundle => { bundle.models.find(model => model.id === 'coverage').data.cells.pop(); } },
  { id: 'terminal_transition_changed', mutate: bundle => { bundle.models.find(model => model.id === 'state').data.edges.find(edge => edge.source === 'DONE').target = 'IMPLEMENT'; } },
];

await fs.mkdir(reportDir, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const report = {
  category: 'review_loader_source_boundary', status: 'running', recorded_at: new Date().toISOString(),
  browser_url: browserURL, browser_path: 'Playwright: Browser plugin not available; CUA iab unavailable and inventory timed out',
  viewport: { width: 1440, height: 1000 }, scope: 'Public static review loader rejection; no Agent API writes or v2/live claim',
  checks: [], probes: [], console: [], runtime_requests: [], screenshots: [],
};
const reviewURL = new URL(browserURL); reviewURL.searchParams.set('review', 'state');
const publicURL = filename => new URL(`./review/${filename}`, browserURL).href;

try {
  const requestContext = await browser.newContext();
  const [bundleResponse, schemaResponse] = await Promise.all([
    requestContext.request.get(publicURL('bundle.json')), requestContext.request.get(publicURL('schema.json')),
  ]);
  if (!bundleResponse.ok() || !schemaResponse.ok()) throw new Error('Public review publication unavailable');
  const bundleText = await bundleResponse.text(), schemaText = await schemaResponse.text();
  const original = JSON.parse(bundleText), schema = JSON.parse(schemaText);
  report.publication = { bundle_sha256: digest(bundleText), schema_sha256: digest(schemaText), generated_at: original.generated_at, baseline: original.baseline };
  await requestContext.close();

  const schemaProbe = { id: 'network_schema_unknown_draft', error: '发布 Schema 摘要与当前构建不一致', probe: true, schemaOnly: true, mutate: (_bundle, remoteSchema) => { remoteSchema.$schema = 'https://example.invalid/unknown-schema'; } };
  for (const test of [{ id: 'valid_publication_control' }, ...cases.map(test => ({ ...test, error: publicationMismatch })), schemaProbe]) {
    const context = await browser.newContext({ viewport: report.viewport });
    const page = await context.newPage(); page.setDefaultTimeout(15000);
    page.on('pageerror', error => report.console.push({ case_id: test.id, type: 'pageerror', message: String(error) }));
    page.on('console', message => { if (['error', 'warning'].includes(message.type())) report.console.push({ case_id: test.id, type: message.type(), message: message.text() }); });
    page.on('request', request => { const url = new URL(request.url()); if (url.pathname.startsWith('/api/')) report.runtime_requests.push({ case_id: test.id, method: request.method(), path: url.pathname }); });
    if (test.mutate) {
      const altered = structuredClone(original), alteredSchema = structuredClone(schema);
      test.mutate(altered, alteredSchema);
      const remoteBundleText = test.schemaOnly ? bundleText : JSON.stringify(altered);
      await page.route('**/review/bundle.json', route => route.fulfill({ status: 200, contentType: 'application/json', body: remoteBundleText }));
      const remoteSchemaText = JSON.stringify(alteredSchema) === JSON.stringify(schema) ? schemaText : JSON.stringify(alteredSchema);
      await page.route('**/review/schema.json', route => route.fulfill({ status: 200, contentType: 'application/json', body: remoteSchemaText }));
    }
    await page.goto(reviewURL.href, { waitUntil: 'networkidle' });
    const canvas = page.getByTestId('review-canvas').locator('svg.dg-svg');
    let alert = '';
    if (test.error) {
      await page.getByRole('alert').waitFor(); alert = (await page.getByRole('alert').innerText()).trim();
      if (!alert.includes(test.error) || await canvas.count() !== 0) throw new Error(`${test.id}: expected rejection, got ${alert}`);
    } else {
      await canvas.waitFor();
      if (await page.getByRole('alert').count() !== 0) throw new Error('Valid publication rejected');
    }
    if (['valid_publication_control', 'baseline_invariants_false', 'network_schema_relaxed_with_extra', 'schema_source_text_resigned'].includes(test.id)) {
      const screenshot = path.join(reportDir, test.id + '.png'); await page.screenshot({ path: screenshot }); report.screenshots.push(screenshot);
    }
    (test.probe ? report.probes : report.checks).push({ name: test.id, passed: true, expected: test.error ? 'rejected_no_diagram' : 'accepted_diagram', alert });
    await context.close();
  }
  if (report.console.length || report.runtime_requests.length) throw new Error('Unexpected console error or runtime API consumption');
  report.status = 'passed';
} catch (error) {
  report.status = 'failed'; report.error = String(error); process.exitCode = 1;
} finally {
  report.completed_at = new Date().toISOString();
  await fs.writeFile(path.join(reportDir, 'source-bound-summary.json'), JSON.stringify(report, null, 2));
  await browser.close();
  console.log(JSON.stringify({ status: report.status, checks: report.checks.length, probes: report.probes.length, report: path.join(reportDir, 'source-bound-summary.json'), error: report.error }));
}
