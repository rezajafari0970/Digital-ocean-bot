#!/usr/bin/env python3
"""Bounded live acceptance; temporary administrator only, no cloud mutations.

Uses the same short-lived operator fixture as cmd/admin-acceptance. Existing
administrators, accounts, proxy policies and credentials are never changed.
Run only after the verified release is installed. All evidence stays root-only.
"""
import argparse
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import runpy
import secrets
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

BASE = 'http://127.0.0.1:28080'
ROOT = Path('/root/dob-provider-browser-20261103')
EXPECTED = '1081be7edc22bb0cd50ccfc93852aa1650f73406'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--country', help='Explicit country for disposable test sessions only; saved account policy is unchanged')
    parser.add_argument('--revision', default=EXPECTED, help='Exact installed public release required for acceptance')
    args = parser.parse_args()
    assert len(args.revision) == 40 and all(c in '0123456789abcdef' for c in args.revision)
    if args.country:
        args.country = args.country.upper()
        assert len(args.country) == 2 and args.country.isascii() and args.country.isalpha()
    os.umask(0o077)
    helpers = runpy.run_path('/opt/dob-production/support/bootstrap.py')
    env = helpers['database_environment'](helpers['read_environment'](Path('/etc/dob-production/env')))
    env['PGOPTIONS'] = '-c statement_timeout=15000 -c lock_timeout=5000'

    def sql(statement):
        result = subprocess.run(['psql', '-XqAt', '-v', 'ON_ERROR_STOP=1'],
                                input=statement, env=env, capture_output=True, text=True, timeout=20)
        if result.returncode:
            raise RuntimeError('acceptance SQL failed; private fixture receipt identifies cleanup target')
        return result.stdout.strip()

    user = str(uuid.uuid4())
    token = secrets.token_hex(32)
    sessions = []
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    result = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'temporary_operator_fixture': user, 'cloud_mutations': 0, 'sessions': []}

    def request(method, path, payload=None, authorized=True):
        headers = {'Content-Type': 'application/json'}
        if authorized:
            headers['Authorization'] = 'Bearer ' + token
        req = urllib.request.Request(BASE + path, method=method, headers=headers,
                                     data=None if payload is None else json.dumps(payload).encode())
        try:
            response = opener.open(req, timeout=100)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.loads(response.read(8 << 20) or b'null')

    def save():
        (ROOT / 'live-browser-acceptance.json').write_text(json.dumps(result, indent=2) + '\n')

    def frame_summary(frame, number):
        image = base64.b64decode(frame.get('image', ''), validate=True)
        assert frame.get('isolated') is True and len(image) > 1000, 'isolated browser frame missing'
        (ROOT / ('provider-session-' + str(number) + '.jpg')).write_bytes(image)
        parsed = urllib.parse.urlsplit(frame.get('url', ''))
        return {'isolated': True, 'image_bytes': len(image), 'image_sha256': hashlib.sha256(image).hexdigest(),
                'origin': parsed.scheme + '://' + parsed.netloc, 'title': frame.get('title', '')[:160],
                'width': frame.get('width'), 'height': frame.get('height')}

    assert request('GET', '/version', authorized=False)[1]['commit'] == args.revision
    assert request('GET', '/readyz', authorized=False)[1]['status'] == 'ready'
    save()
    try:
        sql("BEGIN; INSERT INTO admin_users(id,username,password_hash,role,must_change_password) VALUES('" + user +
            "','acceptance-" + user + "','!','admin',false); INSERT INTO admin_sessions(id,user_id,token_hash,expires_at) VALUES('" +
            str(uuid.uuid4()) + "','" + user + "',decode('" + hashlib.sha256(token.encode()).hexdigest() +
            "','hex'),now()+interval '10 minutes'); COMMIT;")
        code, providers = request('GET', '/api/v1/providers')
        assert code == 200
        result['registered_provider_count'] = len(providers)
        result['provider_entrypoints'] = {p['name']: p.get('website_url', '') for p in providers}
        assert len(providers) == 7 and all(url.startswith('https://') for url in result['provider_entrypoints'].values())
        code, proxies = request('GET', '/api/v1/proxies')
        assert code == 200
        healthy = {p['id'] for p in proxies if p.get('status') == 'healthy'}
        code, accounts = request('GET', '/api/v1/accounts')
        assert code == 200
        candidates = [a for a in accounts if a.get('provider') == 'vultr']
        candidates += [a for a in accounts if a.get('provider') != 'vultr']
        selected = None
        for account in candidates:
            ids = account.get('proxy_ids') or [account.get('proxy_id')]
            ids = list(dict.fromkeys(p for p in ids if p in healthy))[:3]
            if ids:
                selected = account, ids
                break
        if selected is None:
            result['blocked'] = 'no existing account with an assigned healthy proxy'
            return
        account, ids = selected
        code, policy = request('GET', '/api/v1/accounts/' + account['id'] + '/proxy-country-policy')
        assert code == 200
        policy = policy['policy']
        countries = [c for c in policy.get('countries', []) if c != 'AUTO'][:3]
        if not countries:
            countries = [(policy.get('active_country') or '').upper() or 'US']
        if args.country:
            countries = [args.country]
        result.update(provider=account['provider'], requested_countries=countries,
                      country_selection=('explicit operator test selection; saved account policy unchanged' if args.country else
                                         'saved explicit priorities, otherwise current observed country or US for this disposable session'))
        payload = {'provider': account['provider'], 'account_id': account['id'], 'proxy_id': ids[0],
                   'proxy_ids': ids, 'proxy_countries': countries, 'width': 390, 'height': 800}
        code, rejected = request('POST', '/api/v1/provider-browser/sessions', payload, authorized=False)
        result['unauthenticated_rejected'] = code == 401
        code, rejected = request('POST', '/api/v1/provider-browser/sessions', dict(payload, proxy_id='', proxy_ids=[]))
        result['missing_proxy_rejected'] = code == 400 and rejected.get('error') == 'browser_proxy_required'
        assert result['unauthenticated_rejected'] and result['missing_proxy_rejected']
        save()
        for number in (1, 2):
            code, opened = request('POST', '/api/v1/provider-browser/sessions', payload)
            if code != 201:
                result['blocked'] = {'stage': 'start_session_' + str(number), 'status': code,
                                     'error': opened.get('error'), 'detail': opened.get('detail', '')[:240]}
                return
            session = opened['id']
            sessions.append(session)
            assert opened['country'] in countries and opened['proxy_id'] in ids
            record = {'country': opened['country'], 'exit_ip': opened['exit_ip'],
                      'frame': frame_summary(opened['frame'], number)}
            result['sessions'].append(record)
            save()
            time.sleep(3)
            code, frame = request('POST', '/api/v1/provider-browser/sessions/' + session + '/action', {'kind': 'frame'})
            if code != 200:
                result['blocked'] = {'stage': 'frame_' + str(number), 'status': code, 'error': frame.get('error')}
                return
            record['frame'] = frame_summary(frame, number)
            save()
        # Refresh both after startup so the retained frames include asynchronous
        # page loading. A returned frame alone does not establish provider login.
        time.sleep(5)
        for number, session in enumerate(sessions, 1):
            code, frame = request('POST', '/api/v1/provider-browser/sessions/' + session + '/action', {'kind': 'frame'})
            if code != 200:
                result['blocked'] = {'stage': 'final_frame_' + str(number), 'status': code, 'error': frame.get('error')}
                return
            result['sessions'][number - 1]['frame'] = frame_summary(frame, number)
            save()
        result['distinct_sessions'] = len(set(sessions)) == 2
        units = subprocess.check_output(['systemctl', 'list-units', '--type=service', '--plain', '--no-legend',
                                         'datacenter-provider-browser@*'], text=True).splitlines()
        namespaces = []
        for line in units:
            unit = line.split()[0]
            pid = int(subprocess.check_output(['systemctl', 'show', unit, '-p', 'MainPID', '--value'], text=True))
            if pid:
                process = Path('/proc') / str(pid)
                namespaces.append({'network': os.readlink(process / 'ns/net'),
                                   'mount': os.readlink(process / 'ns/mnt'), 'nonroot': process.stat().st_uid != 0})
        result['active_browser_workers'] = len(namespaces)
        result['live_namespace_isolation'] = (len(namespaces) >= 2 and all(x['nonroot'] for x in namespaces)
            and len({x['network'] for x in namespaces}) == len(namespaces)
            and len({x['mount'] for x in namespaces}) == len(namespaces)
            and all(x['network'] != os.readlink('/proc/1/ns/net') for x in namespaces))
        assert result['live_namespace_isolation'], 'separate nonroot network and mount namespaces required'
        result['status'] = 'two isolated live sessions opened through verified selected-country routes'
    finally:
        closed = []
        for session in sessions:
            try:
                closed.append(request('DELETE', '/api/v1/provider-browser/sessions/' + session)[0] == 204)
            except Exception:
                closed.append(False)
        result['sessions_closed'] = all(closed)
        sql("DELETE FROM admin_users WHERE id='" + user + "' AND username='acceptance-" + user + "';")
        result['temporary_operator_removed'] = sql("SELECT count(*) FROM admin_users WHERE id='" + user + "';") == '0'
        save()
        print(json.dumps(result))


if __name__ == '__main__':
    main()
