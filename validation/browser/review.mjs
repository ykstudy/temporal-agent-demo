import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';
import zlib from 'node:zlib';

// Independent public-UI consumer. This does not import product modules or the
// review-data generator, and it does not assert Agent wire-v2 conformance.
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const browserURL = process.env.AGENT_BROWSER_URL || 'http://127.0.0.1:15181/';
const reportDir = path.resolve(process.env.AGENT_BROWSER_REPORT_DIR || '/tmp/agent-vue-review-qa/review');
await fs.mkdir(path.join(reportDir, 'exports'), { recursive: true });
const routeIDs = ['state', 'responsibility', 'sequence', 'contracts', 'lineage', 'architecture', 'coverage', 'versions', 'knowledge', 'skills', 'context'];
const errors = [];
const runtimeWrites = [];
const report = {
  category: 'review_ui', status: 'running', recorded_at: new Date().toISOString(),
  browser_url: browserURL, browser_path: 'Playwright: Browser plugin not available',
  viewports: [{ width: 1440, height: 1000 }, { width: 390, height: 844 }],
  scope: 'Static design/current/historical evidence review; no v2/live/Run approval claim',
  checks: [], routes: [], screenshots: [], exports: [], console: errors, runtime_writes: runtimeWrites,
};
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const page = await browser.newPage({ viewport: report.viewports[0] });
page.setDefaultTimeout(15000);
page.on('pageerror', error => errors.push({ type: 'pageerror', message: String(error) }));
page.on('console', message => {
  if (['error', 'warning'].includes(message.type())) errors.push({ type: message.type(), message: message.text() });
});
page.on('request', request => {
  const url = new URL(request.url());
  if (url.pathname.startsWith('/api/') && ['POST', 'PUT', 'PATCH', 'DELETE'].includes(request.method())) {
    runtimeWrites.push({ method: request.method(), path: url.pathname });
  }
});
const ok = (name, condition, detail) => {
  report.checks.push({ name, passed: Boolean(condition), ...(detail === undefined ? {} : { detail }) });
  if (!condition) throw new Error(name + (detail === undefined ? '' : ': ' + JSON.stringify(detail)));
};
const reviewURL = id => { const value = new URL(browserURL); value.searchParams.set('review', id); return value.href; };
const canvas = () => page.getByTestId('review-canvas');
const svg = () => canvas().locator('svg.dg-svg[data-review-canvas]');
async function screenshot(name) {
  const filename = path.join(reportDir, name + '.png');
  await page.screenshot({ path: filename });
  report.screenshots.push(filename);
}
async function route(model) {
  await page.goto(reviewURL(model.id), { waitUntil: 'networkidle' });
  await page.getByRole('navigation', { name: '设计审查' }).waitFor();
  await svg().waitFor();
  ok(`route_${model.id}`, new URL(page.url()).searchParams.get('review') === model.id);
  ok(`diagram_${model.id}`, await svg().getAttribute('data-diagram') === model.diagram_type);
  ok(`meaningful_${model.id}`, (await page.locator('body').innerText()).includes(model.title));
  ok(`no_overlay_${model.id}`, await page.locator('vite-error-overlay').count() === 0);
  const buttons = await svg().getByRole('button').count();
  ok(`interactive_elements_${model.id}`, buttons > 0, { buttons });
  report.routes.push({ id: model.id, diagram_type: model.diagram_type, interactive_elements: buttons });
}
async function noOverflow(name) {
  const dimensions = await page.evaluate(() => ({ content: document.documentElement.scrollWidth, viewport: innerWidth }));
  ok(name, dimensions.content <= dimensions.viewport, dimensions);
}
async function themeProbe() {
  return page.evaluate(() => {
    const canvas = document.querySelector('[data-testid="review-canvas"]');
    const shape = canvas?.querySelector('svg rect');
    return {
      body: getComputedStyle(document.body).backgroundColor,
      canvas: canvas ? getComputedStyle(canvas).backgroundColor : null,
      text: canvas ? getComputedStyle(canvas).color : null,
      shape: shape ? getComputedStyle(shape).fill : null,
    };
  });
}
async function download(label, filename) {
  const next = page.waitForEvent('download');
  await page.getByRole('button', { name: label, exact: true }).click();
  const item = await next;
  const target = path.join(reportDir, 'exports', filename);
  await item.saveAs(target);
  ok(`download_${filename}`, !(await item.failure()) && (await fs.stat(target)).size > 100);
  return target;
}
function metadataMatches(meta, model, bundle) {
  return meta && meta.model_id === model.id && meta.diagram_type === model.diagram_type
    && meta.generated_at === bundle.generated_at && meta.scope === model.scope && meta.status === model.status
    && meta.baseline?.source_sha256 === bundle.baseline.source_sha256
    && meta.baseline?.invariants_sha256 === bundle.baseline.invariants_sha256
    && meta.baseline?.contract_version === bundle.baseline.contract_version;
}
function pngChunks(bytes) {
  if (!bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) throw new Error('Invalid PNG signature');
  const chunks = []; let offset = 8;
  while (offset + 12 <= bytes.length) {
    const length = bytes.readUInt32BE(offset), type = bytes.toString('ascii', offset + 4, offset + 8);
    if (length > 32 * 1024 * 1024 || offset + length + 12 > bytes.length) throw new Error('Invalid PNG chunk bounds');
    const data = bytes.subarray(offset + 8, offset + 8 + length);
    chunks.push({ type, data }); offset += length + 12;
    if (type === 'IEND') break;
  }
  if (!chunks.some(item => item.type === 'IEND')) throw new Error('PNG lacks IEND');
  return chunks;
}
function pngMetadata(chunks) {
  const texts = [];
  for (const { type, data } of chunks) {
    const zero = data.indexOf(0);
    if (zero < 0) continue;
    if (type === 'tEXt') texts.push(data.subarray(zero + 1).toString('utf8'));
    if (type === 'zTXt') texts.push(zlib.inflateSync(data.subarray(zero + 2)).toString('utf8'));
    if (type === 'iTXt') {
      const compressed = data[zero + 1]; let start = zero + 3;
      start = data.indexOf(0, start) + 1; start = data.indexOf(0, start) + 1;
      texts.push((compressed ? zlib.inflateSync(data.subarray(start)) : data.subarray(start)).toString('utf8'));
    }
  }
  return texts.map(text => { try { return JSON.parse(text); } catch { return null; } }).filter(Boolean);
}
async function decodedPixels(bytes) {
  return page.evaluate(async base64 => {
    const image = new Image(); image.src = 'data:image/png;base64,' + base64; await image.decode();
    const canvas = document.createElement('canvas'); canvas.width = 96; canvas.height = 96;
    const context = canvas.getContext('2d'); context.drawImage(image, 0, 0, 96, 96);
    const colors = new Set(), values = context.getImageData(0, 0, 96, 96).data;
    for (let i = 0; i < values.length; i += 4) colors.add([...values.slice(i, i + 4)].join(','));
    return { colors: colors.size, naturalWidth: image.naturalWidth, naturalHeight: image.naturalHeight };
  }, bytes.toString('base64'));
}
async function currentTransform() { return canvas().locator('.review-transform').evaluate(element => element.style.transform); }

try {
  const response = await page.request.get(new URL('/review/bundle.json', browserURL).href);
  ok('public_review_bundle_available', response.ok(), { status: response.status() });
  const bundleBytes = await response.body(), bundleDigest = crypto.createHash('sha256').update(bundleBytes).digest('hex');
  const bundle = JSON.parse(bundleBytes.toString('utf8'));
  await fs.writeFile(path.join(reportDir, 'review-bundle.json'), bundleBytes);
  if (process.env.AGENT_REVIEW_BUNDLE_SHA256) ok('public_bundle_matches_expected_release', bundleDigest === process.env.AGENT_REVIEW_BUNDLE_SHA256, { expected: process.env.AGENT_REVIEW_BUNDLE_SHA256, actual: bundleDigest });
  ok('exact_eleven_model_ids', bundle.models.length === 11 && routeIDs.every(id => bundle.models.some(model => model.id === id)));
  ok('review_publication_version_is_separate', bundle.schema_version === 1 && bundle.baseline.contract_version === '1.0.0');
  ok('source_byte_hashes', bundle.sources.every(source => crypto.createHash('sha256').update(source.text, 'utf8').digest('hex') === source.sha256));
  report.bundle = { schema_version: bundle.schema_version, generated_at: bundle.generated_at, baseline: bundle.baseline, source_count: bundle.sources.length, sha256: bundleDigest };
  for (const id of routeIDs) {
    await route(bundle.models.find(model => model.id === id));
    await screenshot('review-' + id + '-desktop');
  }

  const contracts = bundle.models.find(model => model.id === 'contracts');
  await route(contracts);
  const node = svg().locator('.tt-node[role="button"]').first();
  const nodeLabel = await node.getAttribute('aria-label');
  const nodeId = await node.getAttribute('data-node-id');
  const nodeBinding = contracts.bindings[nodeId];
  ok('node_has_public_binding', Boolean(nodeId) && Boolean(nodeBinding) && nodeBinding.label === nodeLabel && contracts.data.classes.some(item => item.id === nodeId), { id: nodeId, label: nodeLabel });
  await node.click();
  const details = page.getByRole('region', { name: '来源详情', exact: true });
  await details.waitFor();
  ok('node_selection_shows_bound_details', (await details.innerText()).includes(nodeBinding.label));
  const source = bundle.sources.find(item => item.id === nodeBinding.source_ids[0]);
  ok('selected_source_exists', Boolean(source));
  await details.getByRole('button', { name: source.path, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '来源详情', exact: true });
  await dialog.waitFor();
  const firstLine = source.text.split('\n').find(line => line.trim())?.replace(/^#+\s*/, '').trim();
  ok('source_dialog_actual_path_and_text', (await dialog.innerText()).includes(source.path) && (await dialog.innerText()).includes(firstLine));
  await screenshot('review-source-dialog');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  ok('source_dialog_returns_focus', await details.getByRole('button', { name: source.path, exact: true }).evaluate(element => document.activeElement === element));
  const coreEdges = svg().locator('.tt-edge[role="button"]');
  ok('default_core_chain_has_seven_relations', await coreEdges.count() === 7, { actual: await coreEdges.count() });
  for (let i = 0; i < 7; i++) {
    const edge = coreEdges.nth(i), edgeLabel = await edge.getAttribute('aria-label'), edgeId = await edge.getAttribute('data-edge-id');
    const matchingEdges = contracts.data.relationships.filter(item => item.id === edgeId);
    ok('edge_identity_is_unambiguous_' + i, Boolean(edgeId) && matchingEdges.length === 1 && matchingEdges[0].label === edgeLabel, { id: edgeId, label: edgeLabel, matches: matchingEdges.length });
    const edgeBinding = contracts.bindings[matchingEdges[0].id];
    ok('edge_has_public_binding_' + i, Boolean(edgeBinding), { id: matchingEdges[0].id, label: edgeLabel });
    // The visible label is the public control. Do not force a click through a
    // different edge, or substitute a keyboard action when pointer hits fail.
    const edgeText = edge.locator('.tt-edge-label');
    const edgeHit = await edgeText.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      const hit = document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
      return { expected: element.closest('.tt-edge')?.getAttribute('data-edge-id'), actual: hit?.closest('.tt-edge')?.getAttribute('data-edge-id') };
    });
    ok('edge_label_pointer_target_is_own_relation_' + i, edgeHit.expected === edgeHit.actual, edgeHit);
    await edgeText.click();
    ok('edge_selection_shows_bound_details_' + i, (await details.innerText()).includes(edgeBinding.label));
  }

  const beforeTheme = await themeProbe();
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  const afterTheme = await themeProbe();
  ok('theme_changes_rendered_colors', JSON.stringify(beforeTheme) !== JSON.stringify(afterTheme), { beforeTheme, afterTheme });
  await screenshot('review-dark');
  await page.reload({ waitUntil: 'networkidle' }); await svg().waitFor();
  ok('theme_survives_reload', JSON.stringify(await themeProbe()) === JSON.stringify(afterTheme));
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  const boundsBeforeZoom = await svg().boundingBox();
  await page.getByRole('button', { name: '放大', exact: true }).click();
  await page.waitForFunction(before => {
    const bounds = document.querySelector('[data-testid="review-canvas"] svg.dg-svg')?.getBoundingClientRect();
    return bounds && bounds.width > before.width && bounds.height > before.height;
  }, boundsBeforeZoom);
  const boundsAfterZoom = await svg().boundingBox();
  ok('zoom_changes_canvas_scale', boundsAfterZoom.width > boundsBeforeZoom.width && boundsAfterZoom.height > boundsBeforeZoom.height, { boundsBeforeZoom, boundsAfterZoom });
  await page.getByRole('button', { name: '缩小', exact: true }).click();
  await page.getByRole('button', { name: '适应画布', exact: true }).click();
  await page.waitForFunction(() => {
    const wrapper = document.querySelector('[data-testid="review-canvas"]')?.getBoundingClientRect();
    const bounds = document.querySelector('[data-testid="review-canvas"] svg.dg-svg')?.getBoundingClientRect();
    return wrapper && bounds && bounds.width <= wrapper.width + 3 && bounds.height <= wrapper.height + 3;
  });
  const fit = await svg().boundingBox(), wrapper = await canvas().boundingBox();
  ok('fit_contains_full_canvas', fit.width <= wrapper.width + 3 && fit.height <= wrapper.height + 3, { fit, wrapper });
  const background = await canvas().evaluate(element => {
    const bounds = element.getBoundingClientRect(), x = bounds.x + 4, y = bounds.y + 4;
    const hit = document.elementFromPoint(x, y);
    return { x, y, outsideX: bounds.right + 20, outsideY: bounds.y + bounds.height / 2, isBackground: Boolean(hit && element.contains(hit) && !hit.closest('button,[role="button"]')) };
  });
  ok('drag_test_starts_on_canvas_background', background.isBackground, background);
  const beforeDrag = await currentTransform();
  await page.mouse.move(background.x, background.y); await page.mouse.down();
  // Move directly outside without an intermediate in-canvas event. A late
  // capture on first movement would leave the pointer-up outside unobserved.
  await page.mouse.move(background.outsideX, background.outsideY);
  await page.mouse.up();
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  const afterRelease = await currentTransform();
  ok('drag_captures_pointer_before_canvas_exit', afterRelease !== beforeDrag, { beforeDrag, afterRelease });
  await page.mouse.move(background.x + 60, background.y + 60);
  await page.mouse.move(background.x + 90, background.y + 90);
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  ok('drag_release_outside_prevents_stuck_pan', await currentTransform() === afterRelease, { released: afterRelease, returned: await currentTransform() });
  await page.getByRole('button', { name: '适应画布', exact: true }).click();
  await page.waitForFunction(expected => document.querySelector('[data-testid="review-canvas"] .review-transform')?.getAttribute('style')?.includes(expected), beforeDrag);
  ok('fit_resets_dragged_canvas', await currentTransform() === beforeDrag);

  const knowledge = bundle.models.find(model => model.id === 'knowledge');
  await route(knowledge);
  const domain = page.getByRole('combobox', { name: '筛选领域', exact: true });
  const type = page.getByRole('combobox', { name: '筛选实体类型', exact: true });
  const relation = page.getByRole('combobox', { name: '筛选关系类型', exact: true });
  await domain.selectOption({ index: 0 }); await type.selectOption({ index: 0 }); await relation.selectOption({ index: 0 });
  const allNodes = await svg().locator('.relation-node[role="button"]').count();
  const domainOptions = await domain.locator('option').evaluateAll(items => items.map(item => ({ value: item.value, label: item.textContent })));
  const agentDomain = domainOptions.find(item => item.value && /agent/i.test(item.value + ' ' + item.label));
  ok('agent_domain_filter_available', Boolean(agentDomain));
  await domain.selectOption(agentDomain.value);
  const agentNodes = await svg().locator('.relation-node[role="button"]').count();
  ok('domain_filter_changes_graph', agentNodes > 0 && agentNodes < allNodes, { allNodes, agentNodes });
  await type.selectOption('feature');
  const featureNodes = await svg().locator('.relation-node[role="button"]').count();
  ok('entity_type_filter_changes_graph', featureNodes > 0 && featureNodes < agentNodes, { agentNodes, featureNodes });
  await domain.selectOption({ index: 0 }); await type.selectOption({ index: 0 });
  const allEdges = await svg().locator('.relation-edge[role="button"]').count();
  const consumesPredicate = [...new Set(knowledge.data.edges.map(edge => edge.label))].find(label => label === 'consumes' || label.startsWith('consumes '));
  ok('consumes_predicate_published', Boolean(consumesPredicate), { consumesPredicate });
  await relation.selectOption(consumesPredicate);
  const consumedEdges = await svg().locator('.relation-edge[role="button"]').count();
  ok('relation_filter_changes_graph', consumedEdges > 0 && consumedEdges < allEdges, { allEdges, consumedEdges });
  await relation.selectOption({ index: 0 });
  const search = page.getByRole('textbox', { name: '搜索审查项', exact: true });
  const needle = knowledge.data.nodes.find(item => item.kind === 'feature' || item.type === 'feature')?.label;
  ok('search_target_from_public_model', Boolean(needle));
  await search.fill(needle);
  const searchedNodes = await svg().locator('.relation-node[role="button"]').count();
  ok('search_changes_graph', searchedNodes > 0 && searchedNodes < allNodes, { needle, searchedNodes, allNodes });
  const searchedFile = await download('导出 SVG', 'knowledge-searched.svg');
  const searchedMetadata = await page.evaluate(text => JSON.parse(new DOMParser().parseFromString(text, 'image/svg+xml').querySelector('metadata')?.textContent || 'null'), await fs.readFile(searchedFile, 'utf8'));
  ok('search_export_bound_metadata', metadataMatches(searchedMetadata, knowledge, bundle) && searchedMetadata.filters?.search === needle && searchedMetadata.filters?.domain === 'all' && searchedMetadata.filters?.entity_type === 'all' && searchedMetadata.filters?.relation_type === 'all', searchedMetadata.filters);
  report.exports.push({ svg: searchedFile, metadata: searchedMetadata });
  await search.fill('__qa_no_matching_entity__');
  ok('search_empty_state_has_no_nodes', await canvas().locator('.relation-node[role="button"]').count() === 0);
  await search.fill('');
  ok('clear_search_restores_graph', await svg().locator('.relation-node[role="button"]').count() === allNodes);
  await domain.selectOption(agentDomain.value);
  await screenshot('review-knowledge-filtered');

  const coverage = bundle.models.find(model => model.id === 'coverage');
  await route(coverage);
  const cells = svg().getByRole('button');
  ok('coverage_exact_discrete_cells', await cells.count() === coverage.data.cells.length, { expected: coverage.data.cells.length, actual: await cells.count() });
  const cell = coverage.data.cells[0], row = coverage.data.rows.find(item => item.id === cell.row), column = coverage.data.columns.find(item => item.id === cell.column);
  const cellControl = svg().getByRole('button', { name: new RegExp(escapeRegExp(row.id) + '.*' + escapeRegExp(column.label) + '.*' + escapeRegExp(cell.status)) });
  await cellControl.focus(); await cellControl.press('Enter');
  ok('coverage_cell_shows_actual_status', (await details.innerText()).includes(cell.status));
  ok('live_not_run_is_explicit', coverage.data.cells.filter(item => item.column === 'live').every(item => item.status === 'not_run'));
  await screenshot('review-coverage-details');

  await page.goto(reviewURL('__unknown_view__'), { waitUntil: 'networkidle' });
  await svg().waitFor();
  ok('unknown_view_falls_back_to_contracts', await svg().getAttribute('data-diagram') === 'class');
  ok('unknown_view_notice', /未知.*视图|不支持.*视图/.test(await page.locator('body').innerText()));
  await page.getByRole('navigation', { name: '设计审查' }).locator('a[href="?review=state"]').click();
  await page.waitForFunction(() => document.querySelector('[data-review-canvas]')?.getAttribute('data-diagram') === 'state');
  ok('known_navigation_updates_route', new URL(page.url()).searchParams.get('review') === 'state');
  ok('unknown_notice_clears_after_known_navigation', !/未知.*视图|不支持.*视图/.test(await page.locator('body').innerText()));

  await route(contracts);
  const coreExpected = await svg().evaluate(element => ({
    viewBox: [element.viewBox.baseVal.x, element.viewBox.baseVal.y, element.viewBox.baseVal.width, element.viewBox.baseVal.height],
    nodes: element.querySelectorAll('.tt-node').length, edges: element.querySelectorAll('.tt-edge').length,
    edge_paths: element.querySelectorAll('.tt-edge-line').length,
  }));
  ok('core_export_fixture_is_eight_nodes_seven_paths', coreExpected.nodes === 8 && coreExpected.edges === 7 && coreExpected.edge_paths === 7, coreExpected);
  const coreSvgFile = await download('导出 SVG', 'contracts-core.svg');
  const coreSvgInfo = await page.evaluate(text => {
    const document = new DOMParser().parseFromString(text, 'image/svg+xml'), root = document.documentElement;
    return { parseError: document.querySelector('parsererror')?.textContent,
      metadata: JSON.parse(root.querySelector('metadata')?.textContent || 'null'),
      viewBox: root.getAttribute('viewBox')?.trim().split(/[ ,]+/).map(Number),
      nodes: root.querySelectorAll('.tt-node').length, edges: root.querySelectorAll('.tt-edge').length,
      edge_paths: root.querySelectorAll('.tt-edge-line').length };
  }, await fs.readFile(coreSvgFile, 'utf8'));
  ok('core_svg_has_eight_nodes_seven_edges', !coreSvgInfo.parseError && coreSvgInfo.nodes === 8 && coreSvgInfo.edges === 7 && coreSvgInfo.edge_paths === 7, coreSvgInfo);
  ok('core_svg_complete_viewbox', JSON.stringify(coreSvgInfo.viewBox) === JSON.stringify(coreExpected.viewBox));
  ok('core_svg_metadata_current_filter', metadataMatches(coreSvgInfo.metadata, contracts, bundle) && coreSvgInfo.metadata.filters?.all_contracts === false && coreSvgInfo.metadata.filters?.search === '');
  const corePngFile = await download('导出 PNG', 'contracts-core.png');
  const corePngBytes = await fs.readFile(corePngFile), coreChunks = pngChunks(corePngBytes), coreHeader = coreChunks.find(item => item.type === 'IHDR').data;
  const coreWidth = coreHeader.readUInt32BE(0), coreHeight = coreHeader.readUInt32BE(4), corePixels = await decodedPixels(corePngBytes);
  ok('core_png_metadata_current_filter', pngMetadata(coreChunks).some(meta => metadataMatches(meta, contracts, bundle) && meta.filters?.all_contracts === false && meta.filters?.search === ''));
  ok('core_png_complete_canvas_ratio', Math.abs(coreWidth / coreHeight - coreExpected.viewBox[2] / coreExpected.viewBox[3]) <= Math.max(0.002, 2 / coreHeight), { width: coreWidth, height: coreHeight, viewBox: coreExpected.viewBox });
  ok('core_png_decodes_nonblank_diagram', corePixels.colors > 8 && corePixels.naturalWidth === coreWidth && corePixels.naturalHeight === coreHeight, corePixels);
  report.exports.push({ kind: 'core_binding_chain', svg: coreSvgFile, png: corePngFile, width: coreWidth, height: coreHeight, expected: coreExpected, metadata: coreSvgInfo.metadata });
  await page.getByRole('checkbox', { name: '全部 Schema 对象', exact: true }).check();
  await page.waitForFunction(expected => document.querySelectorAll('[data-testid="review-canvas"] svg.dg-svg .tt-node').length === expected, contracts.data.classes.length);
  await page.getByRole('button', { name: '放大', exact: true }).click();
  const expectedExport = await svg().evaluate(element => ({
    viewBox: [element.viewBox.baseVal.x, element.viewBox.baseVal.y, element.viewBox.baseVal.width, element.viewBox.baseVal.height],
    interactive: element.querySelectorAll('[role="button"]').length,
    nodes: element.querySelectorAll('.tt-node').length,
    edges: element.querySelectorAll('.tt-edge').length,
    edge_paths: element.querySelectorAll('.tt-edge-line').length,
    label_leaders: element.querySelectorAll('.tt-label-leader').length,
    viewport: { width: innerWidth, height: innerHeight },
  }));
  ok('export_fixture_exceeds_viewport', expectedExport.viewBox[3] > expectedExport.viewport.height);
  const svgFile = await download('导出 SVG', 'contracts.svg');
  const svgText = await fs.readFile(svgFile, 'utf8');
  const svgInfo = await page.evaluate(text => {
    const document = new DOMParser().parseFromString(text, 'image/svg+xml');
    const root = document.documentElement;
    return {
      parseError: document.querySelector('parsererror')?.textContent,
      metadata: JSON.parse(document.querySelector('metadata')?.textContent || 'null'),
      viewBox: root.getAttribute('viewBox')?.trim().split(/[ ,]+/).map(Number),
      interactive: root.querySelectorAll('[role="button"]').length,
      nodes: root.querySelectorAll('.tt-node').length,
      edges: root.querySelectorAll('.tt-edge').length,
      edge_paths: root.querySelectorAll('.tt-edge-line').length,
      label_leaders: root.querySelectorAll('.tt-label-leader').length,
      labels: root.querySelectorAll('text').length,
    };
  }, svgText);
  ok('svg_export_full_nodes_edges', !svgInfo.parseError && svgInfo.nodes === expectedExport.nodes && svgInfo.edges === expectedExport.edges && svgInfo.edge_paths === expectedExport.edge_paths && svgInfo.edge_paths === svgInfo.edges && svgInfo.edge_paths > 0 && svgInfo.labels > svgInfo.nodes, { svgInfo, expectedExport });
  ok('svg_export_preserves_displaced_label_leaders', svgInfo.label_leaders === expectedExport.label_leaders && svgInfo.label_leaders > 0, { exported: svgInfo.label_leaders, rendered: expectedExport.label_leaders });
  ok('svg_export_full_viewbox', JSON.stringify(svgInfo.viewBox) === JSON.stringify(expectedExport.viewBox));
  ok('svg_export_bound_metadata', metadataMatches(svgInfo.metadata, contracts, bundle) && svgInfo.metadata.filters?.search === '' && svgInfo.metadata.filters?.all_contracts === true);
  const pngFile = await download('导出 PNG', 'contracts.png');
  const pngBytes = await fs.readFile(pngFile), chunks = pngChunks(pngBytes), header = chunks.find(item => item.type === 'IHDR').data;
  const width = header.readUInt32BE(0), height = header.readUInt32BE(4), metas = pngMetadata(chunks);
  ok('png_export_bound_metadata', metas.some(meta => metadataMatches(meta, contracts, bundle) && meta.filters?.search === '' && meta.filters?.all_contracts === true));
  const wholeRatio = expectedExport.viewBox[2] / expectedExport.viewBox[3];
  ok('png_export_full_canvas_ratio', width > 0 && height > 0 && Math.abs(width / height - wholeRatio) <= Math.max(0.002, 2 / height), { width, height, wholeRatio });
  const pixels = await decodedPixels(pngBytes);
  ok('png_decodes_nonblank_diagram', pixels.colors > 8 && pixels.naturalWidth === width && pixels.naturalHeight === height, pixels);
  report.exports.push({ svg: svgFile, png: pngFile, width, height, expected: expectedExport, metadata: svgInfo.metadata });

  await page.setViewportSize(report.viewports[1]);
  for (const id of routeIDs) {
    await page.goto(reviewURL(id), { waitUntil: 'networkidle' }); await svg().waitFor();
    const inspector = page.getByRole('region', { name: '来源详情', exact: true });
    if (await inspector.isVisible()) {
      await inspector.getByRole('button', { name: '关闭审查说明', exact: true }).click();
      await inspector.waitFor({ state: 'hidden' });
    }
    const accessible = await canvas().evaluate(element => {
      const bounds = element.getBoundingClientRect();
      const target = document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
      return Boolean(target && element.contains(target));
    });
    ok('mobile_canvas_not_occluded_' + id, accessible);
    await noOverflow('mobile_no_overflow_' + id);
  }
  await page.getByRole('button', { name: '显示说明', exact: true }).click();
  const mobileInspector = page.getByRole('region', { name: '来源详情', exact: true });
  await mobileInspector.waitFor();
  await mobileInspector.getByRole('button', { name: '关闭审查说明', exact: true }).click();
  await mobileInspector.waitFor({ state: 'hidden' });
  ok('mobile_inspector_opens_and_closes_by_pointer', true);
  await screenshot('review-mobile');
  await page.getByRole('link', { name: '返回工作区', exact: true }).click();
  await page.getByRole('textbox', { name: '项目需求', exact: true }).waitFor();
  ok('mobile_return_reaches_workspace', !new URL(page.url()).searchParams.has('review') && (await page.title()).includes('项目创建 Agent'));
  ok('lazy_workspace_loads_readonly', await page.getByRole('textbox', { name: '项目需求', exact: true }).isVisible());
  await noOverflow('returned_mobile_workspace_no_overflow');
  await screenshot('workspace-readonly-mobile');
  ok('readonly_review_sends_no_runtime_writes', runtimeWrites.length === 0, runtimeWrites);
  ok('console_health', errors.length === 0, errors);
  report.status = 'passed';
} catch (error) {
  report.status = 'failed'; report.error = String(error);
  try { await screenshot('review-failure'); } catch { /* Keep original finding. */ }
  throw error;
} finally {
  await fs.writeFile(path.join(reportDir, 'review-summary.json'), JSON.stringify(report, null, 2));
  await browser.close();
  console.log(JSON.stringify({ status: report.status, category: report.category, checks: report.checks.length, report: path.join(reportDir, 'review-summary.json') }));
}

function escapeRegExp(text) { return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }
