"""S3 oracle over published messages, original JSONL and exact object references."""
import json

from .oracle import byte_hash, canonicalize, strict_loads
from .semantic_checks import require, verify_hash, object_hash, ref_key


def source_text(message):
    out = []
    for part in message['parts']:
        text = part['text'] if part['type'] == 'text' else part['fallback_text']
        if text not in out: out.append(text)
    return '\n'.join(out)


def verify_conversation(audit, oracle, raw, resolve):
    oracle.validate('ConversationAuditBundle', audit)
    verify_hash('ConversationAuditBundle', audit)
    messages = audit['messages']
    require(raw.endswith(b'\n'), 'original transcript must end with LF')
    lines = raw.splitlines(keepends=True)
    require(len(lines) == len(messages), 'original transcript cardinality differs')
    require(byte_hash(raw) == audit['transcript_sha256'], 'original transcript digest differs')
    ids = {}; component_versions = {}

    def references(value):
        if isinstance(value, dict):
            if all(k in value for k in ('object_type','object_id','object_version','source_version','hash')):
                oracle.validate('ObjectReference', value)
                require(resolve(value) is not None, 'reference is not available in this Run')
            else:
                for child in value.values(): references(child)
        elif isinstance(value, list):
            for child in value: references(child)

    for seq, (message, line) in enumerate(zip(messages, lines), 1):
        oracle.validate('MessageEnvelope', message, schema_file='message.schema.json')
        require(message['run_id'] == audit['run_id'] and message['sequence'] == seq, 'wrong message Run or sequence')
        require(message['message_id'] not in ids, 'duplicate message ID')
        require(message['state_sequence'] <= audit['state_sequence'] and message['source_version'] <= audit['source_version'], 'message from future source')
        require(strict_loads(line) == message and canonicalize(message) + b'\n' == line, 'raw bytes do not contain the exact original envelope')
        ids[message['message_id']] = message
        part_ids = set()
        for part in message['parts']:
            require(part['part_id'] not in part_ids, 'duplicate Part identity'); part_ids.add(part['part_id'])
            references(part)
            if part['type'] == 'component':
                key = (part['component_id'], part['component_version']); body = canonicalize(part)
                require(key not in component_versions or component_versions[key] == body, 'component identity reused with other bytes')
                component_versions[key] = body

    def check_source(source):
        first, last = source['first_sequence'], source['last_sequence']
        require(1 <= first <= last <= len(messages), 'summary range outside original transcript')
        selected = messages[first-1:last]
        require(source['message_ids'] == [m['message_id'] for m in selected], 'summary message range changed')
        require(source['first_message_id'] == selected[0]['message_id'] and source['last_message_id'] == selected[-1]['message_id'], 'summary endpoints differ')
        require(source['raw_jsonl_sha256'] == byte_hash(b''.join(lines[first-1:last])), 'summary original-byte digest differs')
        return selected

    previous = None; summaries = {}
    for version, summary in enumerate(audit['summaries'], 1):
        oracle.validate('SummaryRecord', summary); verify_hash('SummaryRecord', summary)
        require(summary['run_id'] == audit['run_id'] and summary['definition_snapshot_hash'] == audit['definition_snapshot_hash'] and summary['summary_version'] == version, 'summary identity differs')
        selected = check_source(summary['source'])
        require(summary['content_sha256'] == byte_hash(summary['content'].encode()), 'summary content digest differs')
        require(summary['previous_summary_ref'] == previous, 'summary predecessor changed')
        ref = dict(object_type='summary', object_id=summary['summary_id'], object_version=version, source_version=summary['source_version'], hash=summary['summary_hash'])
        require(resolve(ref) == summary, 'summary historical body differs')
        previous = ref; summaries[ref_key(ref)] = summary
        # The generator publishes quoted source fragments. Independently check
        # every fragment instead of trusting a self-reported summary hash.
        content_lines = summary['content'].splitlines()
        require(content_lines[0] == 'Derived source excerpts; not authority.', 'derived summary marker missing')
        allowed = {m['message_id']:m for m in selected}; quoted = set()
        for line in content_lines[1:]:
            label, quote = line.split('] ', 1); mid, role = label[1:].split(' ', 1)
            require(mid in allowed and role == allowed[mid]['role'] and mid not in quoted, 'summary quote identity invalid')
            excerpt = json.loads(quote); require(excerpt in source_text(allowed[mid]), 'summary invented source text'); quoted.add(mid)
        for m in selected:
            text = source_text(m).lower()
            if any(word in text for word in ('失败','阻塞','待确认','错误','failed','blocked','error','unknown','pending approval')):
                require(m['message_id'] in quoted, 'summary lost unresolved source')

    for attempt in audit['summary_attempts']:
        oracle.validate('SummaryAttempt', attempt); verify_hash('SummaryAttempt', attempt); check_source(attempt['source'])
        require(attempt['run_id'] == audit['run_id'] and attempt['definition_snapshot_hash'] == audit['definition_snapshot_hash'], 'summary attempt source differs')
        if attempt['status'] == 'succeeded': require(ref_key(attempt['summary_ref']) in summaries, 'attempt references missing summary')
    receipt_ids = set()
    for receipt in audit['command_receipts']:
        oracle.validate('CommandReceipt', receipt)
        require(receipt['run_id'] == audit['run_id'] and receipt['command_id'] not in receipt_ids, 'duplicate or foreign receipt'); receipt_ids.add(receipt['command_id'])
    return {'messages':len(messages),'summaries':len(summaries),'summary_attempts':len(audit['summary_attempts']),'receipts':len(receipt_ids),'raw_sha256':byte_hash(raw)}
