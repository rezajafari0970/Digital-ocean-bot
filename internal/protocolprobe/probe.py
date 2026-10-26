"""Bounded, node-local native-driver UDP probes; credentials use stdin only.

External SOCKS requires fresh DNS and two QUIC version-negotiation exchanges.
Managed TLS/XUDP requires two permitted-domain QUIC replies; DNS is not probed.
This measures datagram reachability, not HTTP/3 throughput.
No production configuration is read or changed and no public listener opens.
"""
import concurrent.futures
import errno
import hashlib
import json
import os
import pathlib
import platform
import signal
import socket
import struct
import subprocess
import sys
import time

DNS_QUESTION = b"\x07example\x03com\x00\x00\x01\x00\x01"


def read_exact(sock, count):
    result = b""
    while len(result) < count:
        part = sock.recv(count - len(result))
        if not part:
            raise ValueError("short_reply")
        result += part
    return result


def dns_request():
    identity = os.urandom(2)
    return identity + b"\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00" + DNS_QUESTION


def dns_valid(reply, request):
    if len(reply) < 12 + len(DNS_QUESTION):
        return False
    flags, qd, an = struct.unpack("!HHH", reply[2:8])
    return (reply[:2] == request[:2] and flags & 0x8000 != 0
            and flags & 0x020f == 0 and qd == 1 and an > 0
            and reply[12:12 + len(DNS_QUESTION)] == DNS_QUESTION)


def quic_request():
    dcid, scid = os.urandom(8), os.urandom(8)
    # Reserved version, so this never sends a TLS/application request.
    packet = b"\xc0\xda\xda\xda\xda\x08" + dcid + b"\x08" + scid
    return packet + bytes(1200 - len(packet))


def quic_valid(reply, request):
    if len(reply) < 27 or not reply[0] & 0x80 or reply[1:5] != bytes(4):
        return False
    # RFC 9000: VN swaps the client's source/destination connection IDs.
    if reply[5] != 8 or reply[6:14] != request[15:23]:
        return False
    if reply[14] != 8 or reply[15:23] != request[6:14]:
        return False
    versions = reply[23:]
    return len(versions) % 4 == 0 and any(
        versions[i:i + 4] not in (bytes(4), request[1:5])
        for i in range(0, len(versions), 4))


def datagram_payload(data, destination, port):
    if len(data) < 7 or data[:3] != b"\x00\x00\x00":
        return None
    try:
        packed = socket.inet_aton(destination)
    except OSError:
        packed = None
    if data[3] == 1:
        if len(data) < 10 or (packed is not None and data[4:8] != packed):
            return None
        offset = 8
    elif data[3] == 3:
        length = data[4]
        if len(data) < 7 + length or data[5:5+length] != destination.encode("ascii"):
            return None
        offset = 5 + length
    else:
        return None
    if struct.unpack("!H", data[offset:offset+2])[0] != port:
        return None
    # Domain routes may return the egress-resolved IPv4. The connected local
    # relay socket, destination port and fresh unpredictable QUIC CIDs bind the
    # response without imposing the controller's DNS answer on remote egress.
    return data[offset+2:]


def exchange(local_port, destination, port, payload, validator):
    with socket.create_connection(("127.0.0.1", local_port), timeout=2) as tcp, socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as udp:
        tcp.settimeout(2)
        udp.bind(("127.0.0.1", 0))
        udp.settimeout(2)
        tcp.sendall(b"\x05\x01\x00")
        if read_exact(tcp, 2) != b"\x05\x00":
            return False
        tcp.sendall(b"\x05\x03\x00\x01" + socket.inet_aton("127.0.0.1") + struct.pack("!H", udp.getsockname()[1]))
        header = read_exact(tcp, 4)
        if header != b"\x05\x00\x00\x01":
            return False
        relay = socket.inet_ntoa(read_exact(tcp, 4))
        relay_port = struct.unpack("!H", read_exact(tcp, 2))[0]
        if relay not in ("127.0.0.1", "0.0.0.0") or not relay_port:
            return False
        udp.connect(("127.0.0.1", relay_port))
        try:
            address = b"\x01" + socket.inet_aton(destination)
        except OSError:
            name = destination.encode("ascii")
            if not 1 <= len(name) <= 253:
                return False
            address = b"\x03" + bytes([len(name)]) + name
        udp.send(b"\x00\x00\x00" + address + struct.pack("!H", port) + payload)
        end = time.monotonic() + 2
        while time.monotonic() < end:
            udp.settimeout(max(.01, end - time.monotonic()))
            data = datagram_payload(udp.recv(4096), destination, port)
            if data is not None and validator(data, payload):
                return True
        return False


def sha_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for part in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(part)
    return digest.hexdigest()



def reserve_loopback_pair():
    """Reserve the same TCP/UDP port, retrying only address collisions."""
    for attempt in range(32):
        tcp = udp = None
        try:
            tcp = socket.socket()
            tcp.bind(("127.0.0.1", 0))
            udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            udp.bind(tcp.getsockname())
            return tcp, udp
        except BaseException as error:
            if udp is not None:
                udp.close()
            if tcp is not None:
                tcp.close()
            if not isinstance(error, OSError) or error.errno != errno.EADDRINUSE or attempt == 31:
                raise


def probe(request):
    candidates = request["candidates"]
    if not isinstance(candidates, list) or not 1 <= len(candidates) <= 64:
        raise ValueError("candidates")
    arch = "arm64" if platform.machine() in ("aarch64", "arm64") else "amd64"
    core = pathlib.Path("/usr/local/x-ui/bin/xray-linux-" + arch)
    if sha_file(core) not in request["core_hashes"]:
        raise ValueError("core")
    ports, held, inbounds, outbounds, rules = [], [], [], [], []
    # Default deny also covers an accidentally missing rule.
    outbounds.append({"tag": "deny", "protocol": "blackhole", "settings": {}})
    child = None
    try:
        for i, candidate in enumerate(candidates):
            outbound = candidate["outbound"]
            if outbound.get("protocol") != "socks" or candidate["driver"] not in ("socks5", "managed_tls_xudp"):
                raise ValueError("driver")
            tcp, udp = reserve_loopback_pair()
            held.extend((tcp, udp))
            port = tcp.getsockname()[1]
            ports.append(port)
            tag = "udp-proof-" + str(i)
            inbounds.append({"tag": tag, "listen": "127.0.0.1", "port": port, "protocol": "socks",
                             "settings": {"auth": "noauth", "udp": True, "ip": "127.0.0.1"}})
            outbounds.append(dict(outbound, tag=tag))
            rules.append({"type": "field", "inboundTag": [tag], "outboundTag": tag})
        config = {"log": {"loglevel": "none"}, "inbounds": inbounds, "outbounds": outbounds,
                  "routing": {"domainStrategy": "AsIs", "rules": rules}}
        for sock in held:
            sock.close()
        held.clear()
        child = subprocess.Popen([str(core), "run", "-config", "stdin:"], stdin=subprocess.PIPE,
                                 stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        child.stdin.write(json.dumps(config, separators=(",", ":")).encode())
        child.stdin.close()
        deadline = time.monotonic() + 4
        for port in ports:
            while True:
                if child.poll() is not None:
                    raise ValueError("startup")
                try:
                    with socket.create_connection(("127.0.0.1", port), timeout=.1):
                        break
                except OSError:
                    if time.monotonic() > deadline:
                        raise ValueError("startup")
                    time.sleep(.02)

        def one(item):
            i, candidate = item
            driver = candidate["driver"]
            managed = driver == "managed_tls_xudp"
            # A managed donor denies generic DNS/IP traffic by design. Probe
            # its permitted domain through TLS/XUDP, with no HTTP/ad request.
            destination = "googleads.g.doubleclick.net" if managed else "1.1.1.1"
            tests = [] if managed else [(53, dns_request(), dns_valid)]
            tests += [(443, quic_request(), quic_valid), (443, quic_request(), quic_valid)]
            checks = []
            for dest_port, payload, validate in tests:
                try:
                    checks.append(exchange(ports[i], destination, dest_port, payload, validate))
                except (OSError, ValueError):
                    checks.append(False)
            dns = False if managed else checks[0]
            quic = all(checks if managed else checks[1:])
            capable = quic and (managed or dns)
            return {"id": candidate["id"], "driver": driver, "digest": candidate["digest"],
                    "dns": dns, "quic": quic, "code": "ok" if capable else "datagram_unverified"}


        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(one, enumerate(candidates)))
        if child.poll() is not None:
            raise ValueError("core_stopped")
        return {"results": results}
    finally:
        for sock in held:
            sock.close()
        if child is not None:
            if child.poll() is None:
                child.terminate()
            try:
                child.wait(timeout=2)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=2)


def main():
    # GNU timeout bounds the entire process group even if SSH disconnects.
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(124))
    try:
        result = probe(json.loads(sys.stdin.read(256 * 1024 + 1)))
    except Exception:
        result = {"error": "probe_unavailable"}
    print(json.dumps(result, separators=(",", ":")))


if __name__ == "__main__":
    main()
