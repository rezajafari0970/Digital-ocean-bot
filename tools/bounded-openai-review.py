#!/usr/bin/env python3
"""Receipt-based Astra review: one POST, durable budget, independently resumable GETs."""
import argparse
from contextlib import contextmanager
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import tempfile
import time
import urllib.request
import uuid

MODEL = 'gpt-6-astra'
EFFORTS = {'low', 'medium', 'high', 'xhigh', 'max'}
INPUT_MICRO = 13  # Ceiling for the pinned $12.50/M 30-minute cache-write rate.
OUTPUT_MICRO = 50
MAX_INPUT_BYTES = 64000
MAX_OUTPUT = 16384
BUDGET_MICRO = 2000000
MAX_REQUESTS = 3
LEDGER = Path('/var/lib/datacenter-bot-audit/openai/ledger.json')
POLICY = {'model': MODEL, 'budget_micro_usd': BUDGET_MICRO,
          'max_requests': MAX_REQUESTS, 'input_micro_per_token': INPUT_MICRO,
          'output_micro_per_token': OUTPUT_MICRO, 'cache_ttl': '30m'}
PENDING = {'queued', 'in_progress'}
TERMINAL = {'completed', 'failed', 'cancelled', 'incomplete'}
RESPONSE_ID = re.compile(r'resp_[A-Za-z0-9_-]{1,200}\Z')
DIGEST = re.compile(r'[0-9a-f]{64}\Z')
ATTEMPT = re.compile(r'[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}\Z')


class ReviewBlocked(RuntimeError):
    pass


def atomic_write(path, data):
    fd, name = tempfile.mkstemp(prefix='.budget-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            os.fchmod(stream.fileno(), 0o600)
            json.dump(data, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def private_json(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd) as stream:
        info = os.fstat(stream.fileno())
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid()
                or info.st_mode & 0o077 or info.st_size > 5000000):
            raise ReviewBlocked('unsafe_private_file')
        try:
            return json.load(stream)
        except ValueError:
            raise ReviewBlocked('private_json_unreadable') from None


def validate(state, schema=2):
    if (not isinstance(state, dict) or state.get('schema') != schema
            or state.get('policy') != POLICY or not isinstance(state.get('requests'), list)):
        raise ReviewBlocked('budget_policy_mismatch')
    rows = state['requests']
    if len(rows) > MAX_REQUESTS:
        raise ReviewBlocked('budget_ledger_invalid')
    attempts = set()
    for row in rows:
        allowed = {'reserved', 'unknown', 'completed'} if schema == 1 else {'reserved', 'unknown', 'pending', 'settled'}
        if (not isinstance(row, dict) or row.get('status') not in allowed
                or not isinstance(row.get('attempt'), str) or not ATTEMPT.fullmatch(row['attempt'])
                or row['attempt'] in attempts
                or not isinstance(row.get('payload_digest'), str)
                or not DIGEST.fullmatch(row['payload_digest'])
                or type(row.get('reserved_micro_usd')) is not int
                or type(row.get('charged_micro_usd')) is not int
                or not 0 <= row['charged_micro_usd'] <= row['reserved_micro_usd'] <= BUDGET_MICRO
                or row['reserved_micro_usd'] == 0):
            raise ReviewBlocked('budget_ledger_invalid')
        attempts.add(row['attempt'])
        if row.get('reasoning_effort', 'max') not in EFFORTS:
            raise ReviewBlocked('reasoning_effort_invalid')
        if row['status'] in {'reserved', 'unknown', 'pending'} and row['charged_micro_usd'] != row['reserved_micro_usd']:
            raise ReviewBlocked('unresolved_reservation_reduced')
        if schema == 2 and (row['status'] == 'pending' or 'response_id' in row) and not RESPONSE_ID.fullmatch(str(row.get('response_id', ''))):
            raise ReviewBlocked('pending_receipt_invalid')
        if schema == 2 and row.get('protocol') not in {'background-v1', 'legacy-synchronous'}:
            raise ReviewBlocked('unknown_ledger_protocol')
        if row['status'] in {'completed', 'settled'}:
            usage = row.get('usage')
            if (not isinstance(usage, dict) or any(type(usage.get(k)) is not int or usage[k] < 0 for k in ('input_tokens', 'output_tokens'))
                    or row['charged_micro_usd'] != usage['input_tokens'] * INPUT_MICRO + usage['output_tokens'] * OUTPUT_MICRO):
                raise ReviewBlocked('settled_usage_invalid')
        if schema == 2 and row.get('protocol') == 'background-v1':
            if (not DIGEST.fullmatch(str(row.get('prompt_digest', '')))
                    or type(row.get('max_output_tokens')) is not int
                    or not 1 <= row['max_output_tokens'] <= MAX_OUTPUT):
                raise ReviewBlocked('background_record_invalid')
            if row['status'] == 'settled' and (not RESPONSE_ID.fullmatch(str(row.get('response_id', '')))
                    or row.get('response_status') not in TERMINAL
                    or not DIGEST.fullmatch(str(row.get('result_digest', '')))
                    or row.get('result_file') != 'response-' + row['attempt'] + '.json'
                    or row['usage']['output_tokens'] > row['max_output_tokens']):
                raise ReviewBlocked('settled_record_invalid')
    if sum(row['charged_micro_usd'] for row in rows) > BUDGET_MICRO:
        raise ReviewBlocked('ledger_exceeds_budget')


@contextmanager
def locked(path):
    path = Path(path)
    if not path.is_absolute() or path.resolve() != path:
        raise ReviewBlocked('absolute_nonsymlink_ledger_path_required')
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = path.parent.stat()
    if info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise ReviewBlocked('ledger_directory_must_be_private_and_owned')
    fd = os.open(path.with_suffix(path.suffix + '.lock'), os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'a') as lock:
        info = os.fstat(lock.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
            raise ReviewBlocked('unsafe_ledger_lock')
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ReviewBlocked('review_already_running') from None
        state = private_json(path) if path.exists() else {'schema': 2, 'policy': POLICY.copy(), 'requests': []}
        yield path, state


def migrate_ledger(path=LEDGER):
    """Explicit migration preserves every reservation and the original ledger evidence."""
    with locked(path) as (path, state):
        if isinstance(state, dict) and state.get('schema') == 2:
            validate(state)
            return summary(state)
        validate(state, schema=1)
        backup = path.with_name('ledger.schema1.evidence.json')
        if backup.exists():
            if private_json(backup) != state:
                raise ReviewBlocked('migration_evidence_conflict')
        else:
            atomic_write(backup, state)
        for row in state['requests']:
            row['protocol'] = 'legacy-synchronous'
            row['status'] = 'settled' if row['status'] == 'completed' else 'unknown'
        state['schema'] = 2
        validate(state)
        atomic_write(path, state)
        return summary(state)


def summary(state, row=None):
    result = {'model': MODEL, 'reasoning_effort': 'max', 'attempts': len(state['requests']),
              'budget_usd': BUDGET_MICRO / 1e6,
              'accounted_usd_upper': sum(x['charged_micro_usd'] for x in state['requests']) / 1e6,
              'unresolved_reserved_usd': sum(x['charged_micro_usd'] for x in state['requests'] if x['status'] != 'settled') / 1e6}
    if row is not None:
        result.update(attempt=row['attempt'], reasoning_effort=row.get('reasoning_effort', 'max'), status=row['status'], response_status=row.get('response_status'),
                      receipt_saved=bool(row.get('response_id')))
        if row['status'] == 'settled':
            result.update(usage=row.get('usage'), estimated_cost_usd_upper=row['charged_micro_usd'] / 1e6)
    return result


def ledger_status(path=LEDGER):
    with locked(path) as (_, state):
        validate(state)
        result = summary(state)
        result['requests'] = [{k: row.get(k) for k in ('attempt', 'label', 'status', 'response_status', 'reserved_micro_usd', 'charged_micro_usd')} for row in state['requests']]
        return result


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def http_request(method, key, *, payload=None, response_id=None):
    url = 'https://api.openai.com/v1/responses'
    if response_id is not None:
        if not RESPONSE_ID.fullmatch(response_id):
            raise ReviewBlocked('invalid_response_id')
        url += '/' + response_id
    request = urllib.request.Request(url, json.dumps(payload).encode() if payload is not None else None,
                                    {'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json'}, method=method)
    with urllib.request.build_opener(NoRedirect).open(request, timeout=30) as response:
        raw = response.read(5000001)
    if len(raw) > 5000000:
        raise ReviewBlocked('response_too_large')
    return json.loads(raw)


def http_send(payload, key):
    return http_request('POST', key, payload=payload)


def http_fetch(response_id, key):
    return http_request('GET', key, response_id=response_id)


def payload_for(prompt, maximum, *, background=True, reasoning_effort='max'):
    payload = {'model': MODEL, 'reasoning': {'effort': reasoning_effort}, 'max_output_tokens': maximum,
               'input': prompt, 'store': background, 'service_tier': 'default',
               'prompt_cache_options': {'ttl': '30m'}}
    if background:
        payload['background'] = True
    return payload


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


def apply_response(path, state, row, response):
    if (not isinstance(response, dict) or not RESPONSE_ID.fullmatch(str(response.get('id', '')))
            or response.get('id') != row.get('response_id')
            or (response.get('model') != MODEL and not str(response.get('model', '')).startswith(MODEL + '-'))
            or response.get('status') not in PENDING | TERMINAL
            or response.get('service_tier', 'default') != 'default'):
        raise ReviewBlocked('response_identity_or_policy_mismatch')
    row['response_status'] = response['status']
    if response['status'] in PENDING:
        row['status'] = 'pending'
        atomic_write(path, state)
        return summary(state, row)
    usage = response.get('usage')
    if (not isinstance(usage, dict) or any(type(usage.get(k)) is not int or usage[k] < 0 for k in ('input_tokens', 'output_tokens'))):
        raise ReviewBlocked('terminal_usage_missing_or_invalid')
    cost = usage['input_tokens'] * INPUT_MICRO + usage['output_tokens'] * OUTPUT_MICRO
    if cost > row['reserved_micro_usd'] or usage['output_tokens'] > row['max_output_tokens']:
        raise ReviewBlocked('usage_exceeds_reserved_bound')
    artifact = path.with_name('response-' + row['attempt'] + '.json')
    atomic_write(artifact, response)  # Result survives a crash before ledger settlement.
    row.update(status='settled', usage=usage, charged_micro_usd=cost,
               result_file=artifact.name, result_digest=digest(response))
    atomic_write(path, state)
    return summary(state, row)


def submit_review(prompt, key, path=LEDGER, *, max_output_tokens=MAX_OUTPUT,
                  label='review', independent_review=False, reasoning_effort='max', send=http_send):
    if not key:
        raise ReviewBlocked('api_key_missing')
    if not prompt.strip() or len(prompt.encode()) > MAX_INPUT_BYTES:
        raise ReviewBlocked('input_empty_or_too_large')
    if type(max_output_tokens) is not int or not 1 <= max_output_tokens <= MAX_OUTPUT:
        raise ReviewBlocked('output_limit_invalid')
    if not re.fullmatch(r'[a-z0-9][a-z0-9_-]{0,63}', label):
        raise ReviewBlocked('invalid_review_label')
    if reasoning_effort not in EFFORTS:
        raise ReviewBlocked('reasoning_effort_invalid')
    payload = payload_for(prompt, max_output_tokens, reasoning_effort=reasoning_effort)
    signature = digest(payload)
    legacy_signature = digest(payload_for(prompt, MAX_OUTPUT, background=False))
    prompt_digest = hashlib.sha256(prompt.encode()).hexdigest()
    with locked(path) as (path, state):
        validate(state)
        rows = state['requests']
        if any(x['status'] in {'reserved', 'pending'} for x in rows):
            raise ReviewBlocked('submission_or_receipt_already_pending')
        if any(x.get('prompt_digest') == prompt_digest or x['payload_digest'] in {signature, legacy_signature} for x in rows):
            raise ReviewBlocked('duplicate_review')
        if any(x.get('label') == label for x in rows):
            raise ReviewBlocked('review_label_already_used')
        unknown = [x for x in rows if x['status'] == 'unknown']
        # A separately authorized NEW review is not a replay of an unknown
        # operation. Keep every prior reservation at its full worst-case cost,
        # require a distinct prompt/label, and still prefund within the SAME cap.
        # Default behavior and pending-receipt protection remain unchanged.
        if unknown and not independent_review:
            raise ReviewBlocked('unresolved_request_requires_usage_reconciliation')
        if type(independent_review) is not bool:
            raise ReviewBlocked('independent_review_flag_invalid')
        if len(rows) >= MAX_REQUESTS:
            raise ReviewBlocked('request_limit_reached')
        reserve = (len(prompt.encode()) + 2048) * INPUT_MICRO + max_output_tokens * OUTPUT_MICRO
        if sum(x['charged_micro_usd'] for x in rows) + reserve > BUDGET_MICRO:
            raise ReviewBlocked('budget_reservation_exceeds_cap')
        row = {'attempt': str(uuid.uuid4()), 'at': time.time(), 'protocol': 'background-v1',
               'label': label, 'reasoning_effort': reasoning_effort, 'status': 'reserved', 'payload_digest': signature, 'prompt_digest': prompt_digest,
               'max_output_tokens': max_output_tokens, 'reserved_micro_usd': reserve,
               'charged_micro_usd': reserve}
        if independent_review and unknown:
            row['independent_prior_unknowns'] = [x['attempt'] for x in unknown]
        rows.append(row)
        atomic_write(path, state)  # Never POST before the full worst-case cost is durable.
        try:
            response = send(payload, key)
            if not isinstance(response, dict) or not RESPONSE_ID.fullmatch(str(response.get('id', ''))):
                raise ReviewBlocked('submission_receipt_missing')
            row.update(status='pending', response_id=response['id'])
            atomic_write(path, state)  # Save the provider receipt before any subsequent GET.
            return apply_response(path, state, row, response)
        except Exception as exc:
            row.update(status='unknown', charged_micro_usd=reserve, error_type=type(exc).__name__)
            atomic_write(path, state)
            raise ReviewBlocked('submission_outcome_uncertain_reservation_retained') from None


def poll_review(key, path=LEDGER, *, attempt=None, fetch=http_fetch):
    if not key:
        raise ReviewBlocked('api_key_missing')
    with locked(path) as (path, state):
        validate(state)
        rows = [x for x in state['requests'] if x.get('response_id') and (attempt is None or x['attempt'] == attempt)]
        if not rows:
            raise ReviewBlocked('no_resumable_receipt')
        row = rows[-1]
        if row['status'] == 'settled':
            return summary(state, row)
        try:
            artifact = path.with_name('response-' + row['attempt'] + '.json')
            response = private_json(artifact) if artifact.exists() else fetch(row['response_id'], key)
            return apply_response(path, state, row, response)
        except Exception as exc:
            # Retain the known receipt and reservation; next invocation retries GET only.
            row['last_poll_error_type'] = type(exc).__name__
            atomic_write(path, state)
            raise ReviewBlocked('poll_failed_receipt_and_reservation_retained') from None


def review_result(path=LEDGER, *, attempt=None):
    with locked(path) as (path, state):
        validate(state)
        rows = [x for x in state['requests'] if x['status'] == 'settled' and x.get('result_file') and (attempt is None or x['attempt'] == attempt)]
        if not rows:
            raise ReviewBlocked('completed_result_unavailable')
        row = rows[-1]
        expected = 'response-' + row['attempt'] + '.json'
        if row['result_file'] != expected:
            raise ReviewBlocked('unsafe_result_path')
        response = private_json(path.with_name(expected))
        if digest(response) != row['result_digest']:
            raise ReviewBlocked('result_integrity_mismatch')
        result = summary(state, row)
        result['text'] = ''.join(c.get('text', '') for out in response.get('output', []) for c in out.get('content', []) if c.get('type') == 'output_text')
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['migrate', 'status', 'submit', 'poll', 'result'])
    parser.add_argument('--max-output-tokens', type=int, default=MAX_OUTPUT)
    parser.add_argument('--label', default='review')
    parser.add_argument('--attempt')
    parser.add_argument('--reasoning-effort', choices=sorted(EFFORTS), default='max')
    parser.add_argument('--independent-review', action='store_true', help='Explicitly requested new review; never releases or retries prior unknown reservations')
    args = parser.parse_args()
    try:
        if args.command == 'migrate':
            result = migrate_ledger()
        elif args.command == 'status':
            result = ledger_status()
        elif args.command == 'result':
            result = review_result(attempt=args.attempt)
        elif args.command == 'poll':
            result = poll_review(os.environ.get('OPENAI_API_KEY'), attempt=args.attempt)
        else:
            result = submit_review(sys.stdin.read(MAX_INPUT_BYTES + 1), os.environ.get('OPENAI_API_KEY'),
                                   max_output_tokens=args.max_output_tokens, label=args.label,
                                   independent_review=args.independent_review, reasoning_effort=args.reasoning_effort)
    except (ReviewBlocked, OSError) as exc:
        print(json.dumps({'blocked': str(exc) if isinstance(exc, ReviewBlocked) else 'private_io_failed'}), file=sys.stderr)
        return 78
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
