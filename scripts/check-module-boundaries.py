#!/usr/bin/env python3
"""Offline import-boundary audit; never a product startup or DONE dependency."""
import argparse
from copy import deepcopy
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
PREFIX = 'agentstudio/internal/'
ALLOWED = {
    'contracts': set(),
    'definitions': {'contracts'},
    'governance': {'contracts'},
    'storage': {'contracts'},
    'conversation': {'contracts', 'storage'},
    # Composition, legacy adaptation and controlled executors stay together.
    'agent': {'contracts', 'definitions', 'governance', 'storage', 'conversation'},
}


def check(graph):
    for package, dependencies in graph.items():
        owner = package.removeprefix(PREFIX)
        assert owner in ALLOWED, 'new module needs an explicit dependency decision: ' + owner
        for dependency in dependencies:
            assert not any(x in dependency for x in ['/validation', '/design/', 'ai-agent-harness']), 'nonproduct dependency'
            if dependency.startswith(PREFIX):
                assert dependency[len(PREFIX):] in ALLOWED[owner], owner + ' has a reverse product dependency'


def main():
    p = argparse.ArgumentParser(); p.add_argument('--report', type=Path, required=True); args = p.parse_args()
    raw = subprocess.check_output(['go', '-C', str(ROOT/'backend'), 'list', '-f', '{{.ImportPath}} {{join .Imports " "}}', './internal/...'], text=True)
    graph = {line.split()[0]: line.split()[1:] for line in raw.splitlines()}
    check(graph)
    negatives = []
    for owner, dependency in [('contracts', PREFIX+'agent'), ('governance', PREFIX+'storage'), ('conversation', PREFIX+'agent'), ('agent', 'agentstudio/validation')]:
        bad = deepcopy(graph); bad[PREFIX+owner].append(dependency)
        try: check(bad)
        except AssertionError: negatives.append({'owner': owner, 'dependency': dependency, 'rejected': True})
        else: raise AssertionError('boundary mutation accepted')
    sources = {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
               for path in (ROOT/'backend/internal').rglob('*.go') if not path.name.endswith('_test.go')}
    report = dict(passed=True, at=datetime.now(timezone.utc).isoformat(), scope='compiled Go import directions; does not prove runtime authorization or mechanical package extraction', graph=graph, negative_cases=negatives, source_sha256=sources)
    args.report.parent.mkdir(parents=True, exist_ok=True); args.report.write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({'passed': True, 'packages': len(graph), 'negative_cases': len(negatives)}))


if __name__ == '__main__': main()
