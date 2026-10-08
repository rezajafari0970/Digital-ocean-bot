#!/usr/bin/env python3
"""Bounded passive TCP payload byte sample; never prints or stores packet contents."""
import argparse, datetime, ipaddress, json, pathlib, re, select, signal, socket, subprocess, time

p = argparse.ArgumentParser()
p.add_argument('--host', required=True)
p.add_argument('--seconds', type=int, default=900)
p.add_argument('--output', required=True)
a = p.parse_args()
assert 10 <= a.seconds <= 3600
ips = sorted({x[4][0] for x in socket.getaddrinfo(a.host, None, socket.AF_INET, socket.SOCK_STREAM)})
assert ips and all(ipaddress.ip_address(x).version == 4 for x in ips)
route = json.loads(subprocess.check_output(['ip', '-j', 'route', 'get', ips[0]], text=True))[0]
iface = route['dev']
flt = 'tcp and (' + ' or '.join('host ' + x for x in ips) + ')'
cmd = ['tcpdump', '-n', '-q', '-l', '-tt', '-i', iface, flt]
started = time.time()
out = {'started_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'host': a.host,
       'resolved_ipv4': ips, 'interface': iface, 'seconds_requested': a.seconds,
       'metric': 'observed_tcp_payload_bytes_including_retransmissions_not_provider_billing',
       'minutes': {}, 'complete': False, 'dns_scope_limit': 'gateway IPv4 set resolved at sample start'}
capture = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
line_re = re.compile(r'^(\d+\.\d+) IP ([\d.]+)\.\d+ > ([\d.]+)\.\d+: tcp (\d+)')
try:
    while time.time() - started < a.seconds:
        ready, _, _ = select.select([capture.stdout], [], [], 1)
        if not ready:
            if capture.poll() is not None:
                raise RuntimeError('capture exited early')
            continue
        line = capture.stdout.readline()
        m = line_re.match(line)
        if not m:
            continue
        bucket = str(int((float(m[1]) - started) // 60))
        row = out['minutes'].setdefault(bucket, {'tx_bytes': 0, 'rx_bytes': 0, 'packets': 0})
        row['tx_bytes' if m[3] in ips else 'rx_bytes'] += int(m[4])
        row['packets'] += 1
    out['complete'] = True
finally:
    capture.send_signal(signal.SIGINT)
    _, stderr = capture.communicate(timeout=10)
    out['capture_summary'] = stderr[-2000:]
    out['elapsed_seconds'] = time.time() - started
    out['tx_bytes'] = sum(v['tx_bytes'] for v in out['minutes'].values())
    out['rx_bytes'] = sum(v['rx_bytes'] for v in out['minutes'].values())
    pathlib.Path(a.output).write_text(json.dumps(out, indent=2) + '\n')
