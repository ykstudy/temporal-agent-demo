// Independent JavaScript reference serialization: RFC 8785 Appendix A algorithm.
// It tests the published vectors; it is not a frontend runtime validator.
import fs from 'node:fs';
import crypto from 'node:crypto';
const path = new URL('./fixtures/jcs-vectors.json', import.meta.url);
const vectors = JSON.parse(fs.readFileSync(path, 'utf8'));
function canonical(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
  return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical(value[key])).join(',') + '}';
}
for (const entry of vectors.positive) {
  const actual = canonical(JSON.parse(entry.input_json));
  const hash = crypto.createHash('sha256').update(actual).digest('hex');
  if (actual !== entry.expected_canonical || hash !== entry.expected_sha256) throw Error(entry.id);
}
console.log(JSON.stringify({status:'passed', runtime:process.version, scope:'JavaScript positive RFC 8785 vectors only; not strict JSON or product TS validation', vectors:vectors.positive.length, vectors_sha256:crypto.createHash('sha256').update(fs.readFileSync(path)).digest('hex')}));
