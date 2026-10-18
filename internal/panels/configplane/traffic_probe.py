"""Bounded HTTPS proof; retries never substitute for positive TLS/HTTP evidence."""
import http.client
import ssl
import time

def probe_profile_https(port, host, path, diagnostics=None):
 # Only finite codes and attempt counts leave the tunnel. Never return exception
 # text, headers, response bodies or credential-bearing outbound configuration.
 def outcome(code, attempt):
  if diagnostics is not None:
   diagnostics.update(code=code, attempts=attempt+1)
 for attempt in range(2):
  conn = http.client.HTTPSConnection('127.0.0.1', port, timeout=8,
                                    context=ssl.create_default_context())
  try:
   conn.set_tunnel(host, 443)
   conn.request('GET', path, headers={'Host': host, 'Connection': 'close'})
   response = conn.getresponse()
   if 200 <= response.status < 400:
    valid = bool(response.read(1024))
    outcome('ok' if valid else 'empty_body', attempt)
    return valid
   outcome('http_'+str(response.status), attempt)
   if response.status not in (500, 502, 503, 504):
    return False
  except ssl.SSLCertVerificationError:
   outcome('tls_certificate', attempt)
   return False
  except TimeoutError:
   outcome('timeout', attempt)
  except (OSError, http.client.HTTPException):
   outcome('transport', attempt)
  finally:
   conn.close()
  if attempt == 0:
   time.sleep(0.25)
 return False
