"""Bounded HTTPS proof; retries never substitute for positive TLS/HTTP evidence."""
import http.client
import ssl
import time

def probe_profile_https(port, host, path):
 # A fresh tunnel may recover from a transient reset/timeout or upstream 5xx.
 # Permanent HTTP failures and certificate failures are never retried or accepted.
 for attempt in range(2):
  conn = http.client.HTTPSConnection('127.0.0.1', port, timeout=8,
                                    context=ssl.create_default_context())
  try:
   conn.set_tunnel(host, 443)
   conn.request('GET', path, headers={'Host': host, 'Connection': 'close'})
   response = conn.getresponse()
   if 200 <= response.status < 400:
    return bool(response.read(1024))
   if response.status not in (500, 502, 503, 504):
    return False
  except ssl.SSLCertVerificationError:
   return False
  except (OSError, http.client.HTTPException):
   pass
  finally:
   conn.close()
  if attempt == 0:
   time.sleep(0.25)
 return False
